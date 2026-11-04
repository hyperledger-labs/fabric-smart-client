/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"encoding/pem"
	"errors"
	"testing"

	bccsp "github.com/IBM/idemix/bccsp/types"
	m "github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/idemix"
	fabricmsp "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/security"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/sig"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// Idemix test MSPs with distinct issuers, under
// platform/fabric/core/generic/msp/idemix/testdata (compare their
// IssuerPublicKey files).
const (
	issuer1 = "../../core/generic/msp/idemix/testdata/idemix"
	issuer2 = "../../core/generic/msp/idemix/testdata/sameissuer/idemix"
)

// idemixProvider returns an idemix MSP with the credential in dir, named mspID.
func idemixProvider(t *testing.T, dir, mspID string) *idemix.Provider {
	t.Helper()
	d := mem.NewDriver()
	store, err := d.NewKVS("")
	require.NoError(t, err)
	kvss, err := kvs.New(store, "", kvs.DefaultCacheSize)
	require.NoError(t, err)
	auditInfo, err := d.NewAuditInfo("")
	require.NoError(t, err)
	signerInfo, err := d.NewSignerInfo("")
	require.NoError(t, err)
	conf, err := fabricmsp.GetLocalMspConfigWithType(dir, nil, mspID, "idemix")
	require.NoError(t, err)
	provider, err := idemix.NewProviderWithAnyPolicy(conf, kvss, sig.NewService(sig.NewMultiplexDeserializer(), auditInfo, signerInfo))
	require.NoError(t, err)
	return provider
}

// channelIdemixMSP returns the channel-side idemix MSP mspID for the issuer
// in dir, as a channel configuration defines it: issuer material only, no
// credential.
func channelIdemixMSP(t *testing.T, dir, mspID string) fabricmsp.MSP {
	t.Helper()
	conf, err := fabricmsp.GetVerifyingMspConfig(dir, mspID, "idemix")
	require.NoError(t, err)
	conf.Config, err = stripSigner(conf.Config)
	require.NoError(t, err)
	msp, err := fabricmsp.New(&fabricmsp.IdemixNewOpts{Version: fabricmsp.MSPv1_3}, nil)
	require.NoError(t, err)
	require.NoError(t, msp.Setup(conf))
	return msp
}

// stripSigner drops the signer from a serialized IdemixMSPConfig.
func stripSigner(raw []byte) ([]byte, error) {
	conf := &m.IdemixMSPConfig{}
	if err := proto.Unmarshal(raw, conf); err != nil {
		return nil, err
	}
	conf.Signer = nil
	return proto.Marshal(conf)
}

// idemixRequest is callerID's exchange request carrying a fresh pseudonym of
// provider, proven with provider's credential.
func idemixRequest(t *testing.T, provider *idemix.Provider) (view.Identity, *view.Message) {
	t.Helper()
	x, _, err := provider.Identity(nil)
	require.NoError(t, err)
	signer, err := provider.DeserializeSigner(x)
	require.NoError(t, err)
	proof, err := SignRecipientProof(signer, remoteProof(TagExchangeRequest, callerID, x))
	require.NoError(t, err)
	return x, okMsg(t, &ExchangeRecipientRequest{RecipientData: &RecipientData{Identity: x, Proof: proof}})
}

// respondExchange runs the exchange responder on request against a channel
// whose membership is the given MSPs.
func respondExchange(t *testing.T, request *view.Message, msps ...fabricmsp.MSP) (*recipientsEnv, error) {
	t.Helper()
	env := newRecipientsEnv(t, &mockSession{info: view.SessionInfo{Caller: callerID}, recv: newRecv(request)}, nspWithMembership(newTestMembership(t, msps...)))
	_, err := (&RespondExchangeRecipientIdentitiesView{}).Call(env.ctx)
	return env, err
}

// requireRefused checks that the exchange failed on the initiator's proof and
// left no trace but an error reply and one rejection.
func requireRefused(t *testing.T, env *recipientsEnv, err error) {
	t.Helper()
	require.ErrorIs(t, err, ErrRecipientProof)
	require.Empty(t, env.session.sent)
	require.Len(t, env.session.sentErrs, 1)
	require.Empty(t, env.store.bindings)
	require.InDelta(t, 1, env.rejections.count(security.KindRecipientProof), 0)
}

// A node verifies an idemix recipient from any issuer in the channel, also one
// it holds no credential for: its own signer service (testKeys) knows no idemix
// identity at all.
func TestRespondExchange_IdemixFromChannelIssuer(t *testing.T) { //nolint:paralleltest // idemix key import races inside fabric-amcl
	x, req := idemixRequest(t, idemixProvider(t, issuer2, "OrgB"))
	env, err := respondExchange(t, req, channelIdemixMSP(t, issuer1, "OrgA"), channelIdemixMSP(t, issuer2, "OrgB"))
	require.NoError(t, err)
	require.Equal(t, callerID, env.store.bindings[string(x)])
	require.Zero(t, env.rejections.count(security.KindRecipientProof))
}

// An idemix recipient is refused if its issuer is not the one the channel
// configures for its MSP, or its MSP is not in the channel.
func TestRespondExchange_IdemixIssuerNotInChannel(t *testing.T) { //nolint:paralleltest // idemix key import races inside fabric-amcl
	_, req := idemixRequest(t, idemixProvider(t, issuer2, "OrgA"))
	env, err := respondExchange(t, req, channelIdemixMSP(t, issuer1, "OrgA"))
	require.ErrorContains(t, err, "recipient proof: no verifier")
	requireRefused(t, env, err)

	// OrgB is not in the channel, so x is not treated as idemix: the signer
	// service's verifier for it refuses the proof.
	_, req = idemixRequest(t, idemixProvider(t, issuer2, "OrgB"))
	env, err = respondExchange(t, req, channelIdemixMSP(t, issuer1, "OrgA"))
	require.ErrorContains(t, err, "signature verification failed")
	requireRefused(t, env, err)
}

// The local signature service verifies an idemix pseudonym only under its own
// MSP ID: relabelled with the MSP ID of an x509 org of the channel, which
// routes it to the signature service, a pseudonym of the node's local issuer
// is refused, although its credential and proof are valid.
func TestRespondExchange_IdemixRelabelledMSPRefused(t *testing.T) { //nolint:paralleltest // idemix key import races inside fabric-amcl
	provider := idemixProvider(t, issuer1, "OrgA")
	local := sig.NewMultiplexDeserializer()
	local.AddDeserializer(provider)
	d := mem.NewDriver()
	auditInfo, err := d.NewAuditInfo("")
	require.NoError(t, err)
	signerInfo, err := d.NewSignerInfo("")
	require.NoError(t, err)
	sigService := sig.NewService(local, auditInfo, signerInfo)

	nym, _, err := provider.Identity(nil)
	require.NoError(t, err)
	signer, err := provider.DeserializeSigner(nym)
	require.NoError(t, err)
	sID := &m.SerializedIdentity{}
	require.NoError(t, proto.Unmarshal(nym, sID))

	for name, tc := range map[string]struct { //nolint:paralleltest // idemix key import races inside fabric-amcl
		mspID string
		ok    bool
	}{
		"own MSP ID":  {mspID: "OrgA", ok: true},
		"x509 MSP ID": {mspID: "Org1MSP"},
		"unknown MSP": {mspID: "Nobody"},
	} {
		t.Run(name, func(t *testing.T) {
			x := serializedID(t, tc.mspID, sID.IdBytes)
			proof, err := SignRecipientProof(signer, remoteProof(TagExchangeRequest, callerID, x))
			require.NoError(t, err)
			req := okMsg(t, &ExchangeRecipientRequest{RecipientData: &RecipientData{Identity: x, Proof: proof}})
			drv := &testFabricDriverFNS{
				// The channel has no idemix MSP, so every identity goes to sigService.
				channel:    &mockDriverChannel{membership: &testMembership{mgr: fabricmsp.NewMSPManager()}},
				sigService: sigService,
			}
			env := recipientChannelEnv(t, drv, &mockSession{info: view.SessionInfo{Caller: callerID}, recv: newRecv(req)})
			_, err = (&RespondExchangeRecipientIdentitiesView{}).Call(env.ctx)
			if tc.ok {
				// The proof verifies; the view then fails only on signing its
				// own reply, since sigService holds no signer for "me-id".
				require.ErrorContains(t, err, "failed getting signer for recipient identity")
				require.NotErrorIs(t, err, ErrRecipientProof)
				require.Zero(t, env.rejections.count(security.KindRecipientProof))
				return
			}
			require.ErrorContains(t, err, "recipient proof: no verifier")
			requireRefused(t, env, err)
		})
	}
}

// A pseudonym with no credential behind it is refused, although the channel
// MSP's verifier alone would accept its signature: recipientVerifier validates
// the identity first.
func TestRespondExchange_IdemixWithoutCredential(t *testing.T) { //nolint:paralleltest // idemix key import races inside fabric-amcl
	provider := idemixProvider(t, issuer1, "OrgA")
	usk, err := provider.Csp.KeyGen(&bccsp.IdemixUserSecretKeyGenOpts{Temporary: true})
	require.NoError(t, err)
	nym, err := provider.Csp.KeyDeriv(usk, &bccsp.IdemixNymKeyDerivationOpts{Temporary: true, IssuerPK: provider.IssuerPublicKey})
	require.NoError(t, err)
	nymPub, err := nym.PublicKey()
	require.NoError(t, err)
	raw, err := nymPub.Bytes()
	require.NoError(t, err)
	ou, err := proto.Marshal(&m.OrganizationUnit{MspIdentifier: "OrgA"})
	require.NoError(t, err)
	role, err := proto.Marshal(&m.MSPRole{MspIdentifier: "OrgA"})
	require.NoError(t, err)
	idBytes, err := proto.Marshal(&m.SerializedIdemixIdentity{NymX: raw[:len(raw)/2], NymY: raw[len(raw)/2:], Ou: ou, Role: role})
	require.NoError(t, err)
	x, err := proto.Marshal(&m.SerializedIdentity{Mspid: "OrgA", IdBytes: idBytes})
	require.NoError(t, err)

	p := remoteProof(TagExchangeRequest, callerID, x)
	sigma, err := provider.Csp.Sign(usk, p.Payload(), &bccsp.IdemixNymSignerOpts{Nym: nym, IssuerPK: provider.IssuerPublicKey})
	require.NoError(t, err)
	msp := channelIdemixMSP(t, issuer1, "OrgA")
	verifier, err := newTestMembership(t, msp).GetVerifier(x)
	require.NoError(t, err)
	require.NoError(t, VerifyRecipientProof(verifier, p, sigma), "the channel verifier alone accepts the pseudonym")

	req := okMsg(t, &ExchangeRecipientRequest{RecipientData: &RecipientData{Identity: x, Proof: sigma}})
	env, err := respondExchange(t, req, msp)
	require.ErrorContains(t, err, "invalid idemix identity on channel")
	requireRefused(t, env, err)
}

// A recipient proof made by a real idemix pseudonym verifies with the idemix
// verifier for that pseudonym, and only for the context it was made for.
func TestRecipientProof_Idemix(t *testing.T) { //nolint:paralleltest // idemix key import races inside fabric-amcl
	provider := idemixProvider(t, issuer1, "idemix")

	x, _, err := provider.Identity(nil)
	require.NoError(t, err)
	signer, err := provider.DeserializeSigner(x)
	require.NoError(t, err)
	verifier, err := provider.DeserializeVerifier(x)
	require.NoError(t, err)

	p := ProofContext{
		Tag:          TagExchangeRequest,
		SessionID:    "session-1",
		ContextID:    "context-1",
		SignerNode:   view.Identity("alice"),
		VerifierNode: view.Identity("bob"),
		Identity:     x,
	}
	proof, err := SignRecipientProof(signer, p)
	require.NoError(t, err)
	require.NoError(t, VerifyRecipientProof(verifier, p, proof))

	relayed := p
	relayed.VerifierNode = view.Identity("mallory")
	require.ErrorIs(t, VerifyRecipientProof(verifier, relayed, proof), ErrRecipientProof)
}

// recipientChannelEnv returns a network over drv, whose default identity is
// "me-id", for the channel-selection tests.
func recipientChannelEnv(t *testing.T, drv *testFabricDriverFNS, session *mockSession) *recipientsEnv {
	t.Helper()
	drv.localMembership = &testLocalMembership{defaultIdentity: view.Identity("me-id")}
	return newRecipientsEnv(t, session, fabric.NewNetworkServiceProvider(&testFabricDriverFNSProvider{fns: drv}, nil))
}

// serializedID returns a SerializedIdentity of mspID with idBytes, which the
// recipient views route by its MSP on the channel.
func serializedID(t *testing.T, mspID string, idBytes []byte) view.Identity {
	t.Helper()
	raw, err := proto.Marshal(&m.SerializedIdentity{Mspid: mspID, IdBytes: idBytes})
	require.NoError(t, err)
	return raw
}

// Each side verifies with the channel of its own options: the responder
// ignores the Channel the initiator puts in its request.
func TestRecipientExchange_ChannelFromOwnOptions(t *testing.T) {
	t.Parallel()
	ch := &mockDriverChannel{name: "ch", membership: &testMembership{mgr: fabricmsp.NewMSPManager()}}
	x := serializedID(t, "OrgA", []byte("not pem"))

	drv := &testFabricDriverFNS{channel: ch}
	raw, err := (&ExchangeRecipientRequest{Channel: "chosen-by-initiator", RecipientData: &RecipientData{Identity: x, Proof: sign(t, remoteProof(TagExchangeRequest, callerID, x))}}).Bytes()
	require.NoError(t, err)
	env := recipientChannelEnv(t, drv, &mockSession{info: view.SessionInfo{Caller: callerID}, recv: newRecv(&view.Message{Status: view.OK, Payload: raw})})
	_, err = (&RespondExchangeRecipientIdentitiesView{Channel: "mine"}).Call(env.ctx)
	require.NoError(t, err)
	require.Equal(t, []string{"mine"}, drv.channelNames)

	drv = &testFabricDriverFNS{channel: ch}
	env = recipientChannelEnv(t, drv, &mockSession{recv: newRecv(provenReply(t, TagExchangeReply, x))})
	_, err = (&ExchangeRecipientIdentitiesView{Other: view.Identity("other"), Channel: "mine"}).Call(env.ctx)
	require.NoError(t, err)
	require.Equal(t, []string{"mine"}, drv.channelNames)

	drv = &testFabricDriverFNS{channel: ch}
	env = recipientChannelEnv(t, drv, &mockSession{recv: newRecv(provenReply(t, TagRequestReply, x))})
	_, err = (&RequestRecipientIdentityView{Other: view.Identity("other")}).Call(env.ctx)
	require.NoError(t, err)
	require.Equal(t, []string{""}, drv.channelNames, "the default channel")
}

// The option wrappers pass WithChannel to the views.
func TestRecipientExchange_WithChannel(t *testing.T) {
	t.Parallel()
	var got []string
	ctx := &mockViewContext{runViewFn: func(v view.View, _ ...view.RunViewOption) (any, error) {
		switch v := v.(type) {
		case *ExchangeRecipientIdentitiesView:
			got = append(got, v.Channel)
		case *RespondExchangeRecipientIdentitiesView:
			got = append(got, v.Channel)
		}
		return []view.Identity{nil, nil}, nil
	}}
	_, _, err := ExchangeRecipientIdentities(ctx, view.Identity("other"), WithChannel("a"))
	require.NoError(t, err)
	_, _, err = RespondExchangeRecipientIdentities(ctx, WithChannel("b"))
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, got)
}

// unavailableChannels are networks whose channel is unknown, as on a network
// with no default channel, or has no configuration in force yet.
var unavailableChannels = map[string]func() *testFabricDriverFNS{
	"unknown channel": func() *testFabricDriverFNS { return &testFabricDriverFNS{channelErr: errors.New("no such channel")} },
	"config not in force": func() *testFabricDriverFNS {
		return &testFabricDriverFNS{channel: &mockDriverChannel{membership: &testMembership{}}}
	},
}

// A recipient that needs the channel to be verified is refused while the
// channel is unavailable. The refusal is not reported as a rejection of the
// counterparty, and the responder tells the initiator only that the channel
// is not ready.
func TestRecipientExchange_ChannelUnavailable(t *testing.T) {
	t.Parallel()
	x := serializedID(t, "OrgA", []byte("not pem"))
	for name, drv := range unavailableChannels {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			req := okMsg(t, &ExchangeRecipientRequest{RecipientData: &RecipientData{Identity: x, Proof: sign(t, remoteProof(TagExchangeRequest, callerID, x))}})
			env := recipientChannelEnv(t, drv(), &mockSession{info: view.SessionInfo{Caller: callerID}, recv: newRecv(req)})
			_, err := (&RespondExchangeRecipientIdentitiesView{}).Call(env.ctx)
			require.ErrorIs(t, err, errChannelUnavailable)
			require.NotErrorIs(t, err, ErrRecipientProof)
			if name == "config not in force" {
				require.ErrorIs(t, err, fdriver.ErrNotInitialized)
			}
			require.Empty(t, env.session.sent)
			require.Equal(t, [][]byte{[]byte(errChannelUnavailable.Error())}, env.session.sentErrs, "no channel details")
			require.Empty(t, env.store.bindings)
			require.Zero(t, env.rejections.count(security.KindRecipientProof))

			env = recipientChannelEnv(t, drv(), &mockSession{recv: newRecv(provenReply(t, TagRequestReply, x))})
			_, err = (&RequestRecipientIdentityView{Other: view.Identity("other")}).Call(env.ctx)
			require.ErrorIs(t, err, errChannelUnavailable)
			require.Empty(t, env.store.bindings)
		})
	}
}

// An x509 recipient, and an identity that is not a SerializedIdentity, never
// need the channel, so they verify on a network with no default channel and
// on one that is still loading its channel configuration.
func TestRecipientExchange_X509WithoutChannel(t *testing.T) {
	t.Parallel()
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("der")})
	for name, drv := range unavailableChannels {
		for _, x := range []view.Identity{serializedID(t, "Org1MSP", cert), view.Identity("x")} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				d := drv()
				env := recipientChannelEnv(t, d, &mockSession{info: view.SessionInfo{Caller: callerID}, recv: newRecv(okMsg(t, &ExchangeRecipientRequest{RecipientData: &RecipientData{Identity: x, Proof: sign(t, remoteProof(TagExchangeRequest, callerID, x))}}))})
				_, err := (&RespondExchangeRecipientIdentitiesView{}).Call(env.ctx)
				require.NoError(t, err)
				require.Equal(t, callerID, env.store.bindings[string(x)])
				require.Empty(t, d.channelNames, "the channel is not looked up")

				env = recipientChannelEnv(t, drv(), &mockSession{recv: newRecv(provenReply(t, TagRequestReply, x))})
				_, err = (&RequestRecipientIdentityView{Other: view.Identity("other")}).Call(env.ctx)
				require.NoError(t, err)
			})
		}
	}
}
