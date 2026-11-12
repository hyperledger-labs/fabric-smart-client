/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protos_test

import (
	"encoding/json"
	"os/exec"
	"slices"
	"strings"
	"testing"

	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/protos"
)

var testKey = &driver.ChannelKey{Platform: "generic", Channel: "channel", GenesisHash: []byte("genesis")}

// fabricNode is a Fabric transaction manager wired to mocks that let its
// transactions endorse themselves.
type fabricNode struct {
	manager *transaction.Manager
	channel *mock.Channel
	signer  *mock.Signer
}

func newFabricNode(tb testing.TB) *fabricNode {
	tb.Helper()
	n := &fabricNode{channel: &mock.Channel{}, signer: &mock.Signer{}}
	rwset := &mock.RWSet{}
	rwset.BytesReturns([]byte("rwset-bytes"), nil)
	vault := &mock.Vault{}
	vault.NewRWSetReturns(rwset, nil)
	vault.NewRWSetFromBytesReturns(rwset, nil)
	chaincodeManager := &mock.ChaincodeManager{}
	chaincodeManager.ChaincodeReturns(&mock.Chaincode{})
	n.channel.VaultReturns(vault)
	n.channel.MetadataServiceReturns(&mock.MetadataService{})
	n.channel.ChaincodeManagerReturns(chaincodeManager)
	n.channel.ChannelMembershipReturns(&mock.ChannelMembership{})
	provider := &mock.ChannelProvider{}
	provider.ChannelReturns(n.channel, nil)
	n.signer.SignReturns([]byte("sig"), nil)
	sigService := &mock.SignerService{}
	sigService.GetSignerReturns(n.signer, nil)

	n.manager = transaction.NewManager()
	n.manager.AddTransactionFactory(driver.EndorserTransaction, transaction.NewEndorserTransactionFactory("network", provider, sigService))
	return n
}

// newTx returns an unsigned transaction with a chaincode call.
func (n *fabricNode) newTx(tb testing.TB) (*transaction.WrappedTransaction, *transaction.Transaction) {
	tb.Helper()
	tx, err := n.manager.NewTransaction(tb.Context(), driver.EndorserTransaction, []byte("creator"), []byte("nonce"), "txid", "channel", nil)
	require.NoError(tb, err)
	w := tx.(*transaction.WrappedTransaction)
	inner := w.Transaction.(*transaction.Transaction)
	inner.SetProposal("mycc", "1.0", "invoke", "a1", "a2")
	return w, inner
}

// endorsedTx returns a transaction signed and endorsed by "endorser".
func (n *fabricNode) endorsedTx(tb testing.TB) (*transaction.WrappedTransaction, *transaction.Transaction) {
	tb.Helper()
	w, inner := n.newTx(tb)
	require.NoError(tb, inner.SetRWSet())
	require.NoError(tb, inner.EndorseWithIdentity([]byte("endorser")))
	return w, inner
}

// decode converts JSON to an envelope and back, and decodes the result on n.
func (n *fabricNode) decode(t *testing.T, raw []byte) *transaction.Transaction {
	t.Helper()
	tx, err := n.manager.NewTransactionFromBytes(t.Context(), "channel", roundTrip(t, raw))
	require.NoError(t, err)
	return tx.(*transaction.WrappedTransaction).Transaction.(*transaction.Transaction)
}

func roundTrip(t *testing.T, raw []byte) []byte {
	t.Helper()
	env, err := protos.EnvelopeFromJSON(raw)
	require.NoError(t, err)
	require.Equal(t, byte(0x0a), env[0], "an envelope starts with the tag of its platform field")
	back, err := protos.JSONFromEnvelope(env)
	require.NoError(t, err)
	require.Equal(t, byte('{'), back[0])
	again, err := protos.JSONFor(env, "")
	require.NoError(t, err)
	require.Equal(t, back, again, "JSONFor converts an envelope")
	return back
}

func requireSameEndorsement(t *testing.T, want, got []*pb.ProposalResponse) {
	t.Helper()
	require.Len(t, got, len(want))
	for i := range want {
		require.Equal(t, want[i].Payload, got[i].Payload)
		require.True(t, proto.Equal(want[i].Endorsement, got[i].Endorsement))
	}
}

func TestRoundTripFabric(t *testing.T) {
	t.Parallel()
	sender := newFabricNode(t)
	sender.channel.ChannelKeyReturns(testKey, nil)
	w, sent := sender.endorsedTx(t)
	raw, err := w.Bytes()
	require.NoError(t, err)

	got := newFabricNode(t).decode(t, raw)

	require.Equal(t, sent.ID(), got.ID())
	require.Equal(t, sent.TFunction, got.TFunction)
	require.Equal(t, sent.TParameters, got.TParameters)
	require.Equal(t, sent.TTransient, got.TTransient)
	require.Equal(t, sent.TSignedProposal.ProposalBytes, got.TSignedProposal.ProposalBytes)
	require.Equal(t, sent.TSignedProposal.Signature, got.TSignedProposal.Signature)
	requireSameEndorsement(t, sent.TProposalResponses, got.TProposalResponses)

	var outer transaction.SerializedTransaction
	require.NoError(t, json.Unmarshal(roundTrip(t, raw), &outer))
	require.Equal(t, testKey, outer.Key)
	require.Equal(t, driver.EndorserTransaction, outer.Type)
}

func TestDecodesWithFabricProtosAlone(t *testing.T) {
	t.Parallel()
	n := newFabricNode(t)
	w, _ := n.endorsedTx(t)
	raw, err := w.Bytes()
	require.NoError(t, err)
	env, err := protos.EnvelopeFromJSON(raw)
	require.NoError(t, err)

	var msg protos.TransactionEnvelope
	require.NoError(t, proto.Unmarshal(env, &msg))
	require.Equal(t, "generic", msg.GetPlatform())
	require.Equal(t, "channel", msg.GetChannel())
	require.Equal(t, "network", msg.GetNetworkHint())
	var payload protos.Transaction
	require.NoError(t, proto.Unmarshal(msg.GetPayload(), &payload))
	var sp pb.SignedProposal
	require.NoError(t, proto.Unmarshal(payload.GetSignedProposal(), &sp))
	require.NotEmpty(t, sp.ProposalBytes)
	require.Len(t, payload.GetProposalResponses(), 1)
	var pr pb.ProposalResponse
	require.NoError(t, proto.Unmarshal(payload.GetProposalResponses()[0], &pr))
	require.Equal(t, slices.Concat(pr.Payload, pr.Endorsement.Endorser), n.signer.SignArgsForCall(1))
}

func TestUnsignedTransaction(t *testing.T) {
	t.Parallel()
	n := newFabricNode(t)
	w, sent := n.newTx(t)
	require.NoError(t, sent.SetRWSet())
	raw, err := w.Bytes()
	require.NoError(t, err)
	require.Nil(t, sent.TSignedProposal)

	got := newFabricNode(t).decode(t, raw)

	require.Equal(t, "txid", got.ID())
	require.Equal(t, []byte("nonce"), got.Nonce())
	require.Equal(t, []byte("creator"), []byte(got.Creator()))
	require.Equal(t, "mycc", got.Chaincode())
	require.Equal(t, "1.0", got.ChaincodeVersion())
	require.Equal(t, "invoke", got.TFunction)
	require.Equal(t, [][]byte{[]byte("a1"), []byte("a2")}, got.TParameters)
	require.Equal(t, []byte("rwset-bytes"), got.RWSet)
	require.Nil(t, got.TSignedProposal)
}

func TestTransient(t *testing.T) {
	t.Parallel()
	n := newFabricNode(t)
	w, sent := n.newTx(t)
	require.NoError(t, sent.EndorseProposalWithIdentity([]byte("creator")))
	signed := slices.Clone(sent.TSignedProposal.ProposalBytes)
	sent.TTransient["later"] = []byte("w")

	raw, err := w.Bytes()
	require.NoError(t, err)
	got := newFabricNode(t).decode(t, raw)
	require.Equal(t, []byte("w"), got.Transient()["later"])
	require.Equal(t, signed, got.TSignedProposal.ProposalBytes)

	raw, err = w.BytesNoTransient()
	require.NoError(t, err)
	got = newFabricNode(t).decode(t, raw)
	require.NotNil(t, got.Transient())
	require.Empty(t, got.Transient())
	require.Equal(t, signed, got.TSignedProposal.ProposalBytes)
	// endorser.Transaction.SetTransient writes into the decoded map; a nil map would panic.
	require.NoError(t, fabric.TransientMap(got.Transient()).Set("k", []byte("v")))
	require.Equal(t, []byte("v"), got.Transient()["k"])
}

func TestNoKey(t *testing.T) {
	t.Parallel()
	n := newFabricNode(t) // the mock channel has no key
	w, _ := n.endorsedTx(t)
	raw, err := w.Bytes()
	require.NoError(t, err)

	env, err := protos.EnvelopeFromJSON(raw)
	require.NoError(t, err)
	var msg protos.TransactionEnvelope
	require.NoError(t, proto.Unmarshal(env, &msg))
	require.Empty(t, msg.GetChannelGenesisHash())
	require.Equal(t, "generic", msg.GetPlatform())

	var outer transaction.SerializedTransaction
	require.NoError(t, json.Unmarshal(roundTrip(t, raw), &outer))
	require.Nil(t, outer.Key)
	_ = newFabricNode(t).decode(t, raw)
}

func TestDeterministic(t *testing.T) {
	t.Parallel()
	n := newFabricNode(t)
	w, sent := n.endorsedTx(t)
	sent.TTransient = map[string][]byte{"a": []byte("1"), "b": []byte("2"), "c": []byte("3")}
	raw, err := w.Bytes()
	require.NoError(t, err)

	first, err := protos.EnvelopeFromJSON(raw)
	require.NoError(t, err)
	for range 10 {
		again, err := protos.EnvelopeFromJSON(raw)
		require.NoError(t, err)
		require.Equal(t, first, again)
	}
}

func TestRejects(t *testing.T) {
	t.Parallel()
	n := newFabricNode(t)
	w, _ := n.endorsedTx(t)
	raw, err := w.Bytes()
	require.NoError(t, err)
	env, err := protos.EnvelopeFromJSON(raw)
	require.NoError(t, err)
	valid := &protos.TransactionEnvelope{}
	require.NoError(t, proto.Unmarshal(env, valid))

	envelopes := map[string]struct {
		edit func(e *protos.TransactionEnvelope, tx *protos.Transaction)
		want string
	}{
		"empty platform":      {func(e *protos.TransactionEnvelope, _ *protos.Transaction) { e.Platform = "" }, "platform"},
		"unknown platform":    {func(e *protos.TransactionEnvelope, _ *protos.Transaction) { e.Platform = "other" }, "[other]"},
		"no transaction ID":   {func(_ *protos.TransactionEnvelope, tx *protos.Transaction) { tx.TxId = "" }, "no transaction ID"},
		"channel mismatch":    {func(e *protos.TransactionEnvelope, _ *protos.Transaction) { e.Channel = "other" }, "[other]"},
		"bad signed proposal": {func(_ *protos.TransactionEnvelope, tx *protos.Transaction) { tx.SignedProposal = []byte{0xff} }, "signed proposal"},
		"bad response":        {func(_ *protos.TransactionEnvelope, tx *protos.Transaction) { tx.ProposalResponses[0] = []byte{0xff} }, "proposal response"},
	}
	for name, tc := range envelopes {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := proto.CloneOf(valid)
			tx := &protos.Transaction{}
			require.NoError(t, proto.Unmarshal(e.Payload, tx))
			tc.edit(e, tx)
			var err error
			e.Payload, err = proto.Marshal(tx)
			require.NoError(t, err)
			bad, err := proto.Marshal(e)
			require.NoError(t, err)
			_, err = protos.JSONFromEnvelope(bad)
			require.ErrorContains(t, err, tc.want)
		})
	}

	for name, payload := range map[string][]byte{"no payload": nil, "bad payload": {0xff}} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := proto.CloneOf(valid)
			e.Payload = payload
			bad, err := proto.Marshal(e)
			require.NoError(t, err)
			_, err = protos.JSONFromEnvelope(bad)
			require.Error(t, err)
		})
	}

	t.Run("not an envelope", func(t *testing.T) {
		t.Parallel()
		_, err := protos.JSONFromEnvelope([]byte{0xff, 0xff})
		require.Error(t, err)
	})

	jsons := map[string]struct {
		raw  string
		want string
	}{
		"type other than endorser": {`{"Type":1,"Raw":"e30="}`, "type [1]"},
		"unknown key platform":     {`{"TTxID":"tx","TChannel":"channel","Key":{"platform":"other","channel":"channel"}}`, "[other]"},
		"key channel mismatch":     {`{"TTxID":"tx","TChannel":"channel","Key":{"platform":"testflat","channel":"other"}}`, "[other]"},
		"key without platform":     {`{"TTxID":"tx","TChannel":"channel","Key":{"channel":"channel"}}`, "platform"},
		"not json":                 {`nope`, "decode"},
		// JSON the envelope would not keep in full is never turned into a transaction.
		"empty object":           {`{}`, "without a channel key"},
		"unknown field":          {`{"foo":1}`, "without a channel key"},
		"tx with unknown field":  {`{"TTxID":"tx","TChannel":"channel","Extra":1,"Key":{"platform":"testflat","channel":"channel"}}`, "unknown field"},
		"flat tx without id":     {`{"TChannel":"channel","Key":{"platform":"testflat","channel":"channel"}}`, "no transaction ID"},
		"wrapped unknown field":  {`{"Type":3,"Raw":"eyJUVHhJRCI6InR4IiwiRXh0cmEiOjF9"}`, "unknown field"},
		"wrapper unknown field":  {`{"Type":3,"Raw":"eyJUVHhJRCI6InR4In0=","Extra":1}`, "unknown field"},
		"wrapper with tx fields": {`{"Type":3,"Raw":"eyJUVHhJRCI6InR4In0=","TTxID":"tx"}`, "unknown field"},
		"wrapped tx without id":  {`{"Type":3,"Raw":"e30="}`, "no transaction ID"},
		"key inside wrapped tx":  {`{"Type":3,"Raw":"eyJUVHhJRCI6InR4IiwiS2V5Ijp7InBsYXRmb3JtIjoiZ2VuZXJpYyJ9fQ=="}`, "unknown field"},
		"trailing data":          {`{"TTxID":"tx"} {}`, "decode"},
		"wrapped trailing data":  {`{"Type":3,"Raw":"eyJUVHhJRCI6InR4In0ge30="}`, "trailing data"},
		"not an object":          {`[1]`, "decode"},
	}
	for name, tc := range jsons {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := protos.EnvelopeFromJSON([]byte(tc.raw))
			require.ErrorContains(t, err, tc.want)
		})
	}

	// JSONFor passes bytes that are not an envelope to the JSON decoder unchanged.
	for name, raw := range map[string][]byte{"empty": nil, "other first": {0x08, 0x01}} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := protos.JSONFor(raw, "")
			require.NoError(t, err)
			require.Equal(t, raw, got)
		})
	}
}

// testFlat is the platform of flatFormat, a format whose JSON is a flat transaction carrying
// its channel key at the top level, as a platform other than Fabric registers one.
const testFlat = "testflat"

// flatJSON is the JSON of a flatFormat transaction.
type flatJSON struct {
	protos.TxJSON
	Key *driver.ChannelKey `json:",omitempty"`
}

func init() {
	protos.RegisterTransactionFormat(testFlat, protos.TransactionFormat{
		ToPayload: func(raw []byte) ([]byte, *driver.ChannelKey, error) {
			var tx flatJSON
			if err := protos.DecodeStrict(raw, &tx); err != nil {
				return nil, nil, err
			}
			msg, err := tx.ToMessage()
			if err != nil {
				return nil, nil, err
			}
			payload, err := proto.Marshal(msg)
			return payload, tx.Key, err
		},
		ToJSON: func(env *protos.TransactionEnvelope) ([]byte, error) {
			msg := &protos.Transaction{}
			if err := proto.Unmarshal(env.GetPayload(), msg); err != nil {
				return nil, err
			}
			tx, err := protos.TxJSONFromMessage(msg, env)
			if err != nil {
				return nil, err
			}
			return json.Marshal(&flatJSON{TxJSON: *tx, Key: protos.EnvelopeKey(env)})
		},
	})
}

// The Fabric platform does not depend on the platforms built on it (docs/dev/architecture.md).
func TestFabricDoesNotDependOnFabricx(t *testing.T) {
	t.Parallel()
	out, err := exec.CommandContext(t.Context(), "go", "list", "-deps", "-test", "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/...").Output()
	require.NoError(t, err)
	for pkg := range strings.SplitSeq(string(out), "\n") {
		require.NotContains(t, pkg, "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx")
	}
}

func TestRegisterTransactionFormatDuplicatePanics(t *testing.T) {
	t.Parallel()
	require.Panics(t, func() { protos.RegisterTransactionFormat("generic", protos.TransactionFormat{}) })
	require.Panics(t, func() { protos.RegisterTransactionFormat(testFlat, protos.TransactionFormat{}) })
	require.Panics(t, func() { protos.RegisterTransactionFormat("", protos.TransactionFormat{}) })

	f, ok := protos.TransactionFormatOf("generic")
	require.True(t, ok)
	require.False(t, f.SkipTxIDCheck)
	_, ok = protos.TransactionFormatOf("unregistered")
	require.False(t, ok)
}

func TestRegisterTransactionFormatNilFuncsPanics(t *testing.T) {
	t.Parallel()
	toPayload := func(raw []byte) ([]byte, *driver.ChannelKey, error) { return raw, nil, nil }
	toJSON := func(*protos.TransactionEnvelope) ([]byte, error) { return nil, errors.New("not implemented") }
	require.Panics(t, func() {
		protos.RegisterTransactionFormat("testnilpayload", protos.TransactionFormat{ToJSON: toJSON})
	})
	require.Panics(t, func() {
		protos.RegisterTransactionFormat("testniljson", protos.TransactionFormat{ToPayload: toPayload})
	})
	require.Panics(t, func() {
		protos.RegisterTransactionFormat("testnilboth", protos.TransactionFormat{})
	})
}

func TestEnvelopeFromJSONUnregisteredPlatform(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"TTxID":"tx","TChannel":"channel","Key":{"platform":"unregistered","channel":"channel"}}`)
	_, err := protos.EnvelopeFromJSON(raw)
	require.ErrorIs(t, err, protos.ErrNoFormat)
	require.ErrorContains(t, err, "[unregistered]")
	_, err = protos.EnvelopeFromJSONFor([]byte(`{"TTxID":"tx","TChannel":"channel"}`), "unregistered")
	require.ErrorIs(t, err, protos.ErrNoFormat)
	require.ErrorContains(t, err, "[unregistered]")

	env, err := proto.Marshal(&protos.TransactionEnvelope{Platform: "unregistered", Payload: []byte("x")})
	require.NoError(t, err)
	_, err = protos.JSONFromEnvelope(env)
	require.ErrorIs(t, err, protos.ErrNoFormat)
	require.ErrorContains(t, err, "[unregistered]")
}

// A flat transaction without a channel key does not tell its platform.
func TestEnvelopeFromJSONKeylessFlat(t *testing.T) {
	t.Parallel()
	_, err := protos.EnvelopeFromJSON([]byte(`{"TTxID":"tx","TChannel":"channel"}`))
	require.ErrorIs(t, err, protos.ErrNoChannelKey)
	require.ErrorContains(t, err, "cannot tell the platform of a transaction without a channel key")
}

// A sender knows its driver, so a keyless transaction converts for that platform.
func TestEnvelopeFromJSONForKeyless(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"TTxID":"tx","TChannel":"channel"}`)
	env, err := protos.EnvelopeFromJSONFor(raw, testFlat)
	require.NoError(t, err)
	require.Equal(t, byte(0x0a), env[0], "an envelope starts with the platform tag")
	var msg protos.TransactionEnvelope
	require.NoError(t, proto.Unmarshal(env, &msg))
	require.Equal(t, testFlat, msg.GetPlatform())
	require.Empty(t, msg.GetChannelGenesisHash())
	back, err := protos.JSONFor(env, testFlat)
	require.NoError(t, err)
	var flat flatJSON
	require.NoError(t, json.Unmarshal(back, &flat))
	require.Equal(t, "tx", flat.TTxID)
	require.Nil(t, flat.Key)

	keyed := []byte(`{"TTxID":"tx","TChannel":"channel","Key":{"platform":"testflat","channel":"channel","genesisHash":"Zw=="}}`)
	_, err = protos.EnvelopeFromJSONFor(keyed, "generic")
	require.ErrorContains(t, err, "transaction of platform [testflat] on a [generic] network")
	env, err = protos.EnvelopeFromJSONFor(keyed, testFlat)
	require.NoError(t, err)
	again, err := protos.EnvelopeFromJSON(keyed)
	require.NoError(t, err)
	require.Equal(t, env, again)

	_, err = protos.JSONFor(env, "generic")
	require.ErrorContains(t, err, "transaction for platform [testflat] on a [generic] network")
}

// A Fabric SerializedTransaction carries only a Fabric key.
func TestWrapperWithForeignKeyPlatform(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"Type":3,"Raw":"eyJUVHhJRCI6InR4IiwiVENoYW5uZWwiOiJjaGFubmVsIn0=","Key":{"platform":"testflat","channel":"channel"}}`)
	_, err := protos.EnvelopeFromJSON(raw)
	require.ErrorContains(t, err, "[testflat]")
	_, err = protos.EnvelopeFromJSONFor(raw, testFlat)
	require.Error(t, err)
}

// The JSON an envelope of a transaction reads back as is the same transaction as the bytes the
// transaction gave, though not the same bytes: the proposal is left out and the key read back
// from the routing fields. A value that differs elsewhere is not the same transaction.
func TestSameTransactionJSON(t *testing.T) {
	t.Parallel()
	n := newFabricNode(t)
	n.channel.ChannelKeyReturns(testKey, nil)
	w, _ := n.endorsedTx(t)
	endorsed, err := w.Bytes()
	require.NoError(t, err)
	w, _ = newFabricNode(t).newTx(t)
	unsigned, err := w.Bytes()
	require.NoError(t, err)
	for name, raw := range map[string][]byte{"endorsed": endorsed, "unsigned": unsigned} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			env, err := protos.EnvelopeFromJSON(raw)
			require.NoError(t, err)
			back, err := protos.JSONFromEnvelope(env)
			require.NoError(t, err)
			require.NotEqual(t, raw, back)
			require.True(t, protos.SameTransactionJSON(raw, back))
			require.True(t, protos.SameTransactionJSON(back, raw))
		})
	}

	flat := []byte(`{"TTxID":"tx","TChannel":"ch","TParameters":["YQ=="],"Key":{"platform":"testflat","channel":"ch"}}`)
	require.True(t, protos.SameTransactionJSON(flat, []byte(`{"TTxID":"tx","TChannel":"ch","TParameters":["YQ=="],"TTransient":{}}`)), "no genesis hash reads back as no key")

	for name, other := range map[string][]byte{
		"other parameter": []byte(`{"TTxID":"tx","TChannel":"ch","TParameters":["Yg=="]}`),
		"null parameter":  []byte(`{"TTxID":"tx","TChannel":"ch","TParameters":[null]}`),
		"other txID":      []byte(`{"TTxID":"tx2","TChannel":"ch","TParameters":["YQ=="]}`),
		"wrapped":         []byte(`{"Type":3,"Raw":"eyJUVHhJRCI6InR4IiwiVENoYW5uZWwiOiJjaCIsIlRQYXJhbWV0ZXJzIjpbIllRPT0iXX0="}`),
		"unknown field":   []byte(`{"TTxID":"tx","TChannel":"ch","TParameters":["YQ=="],"Extra":1}`),
		"not JSON":        []byte(`nope`),
	} {
		require.False(t, protos.SameTransactionJSON(flat, other), name)
	}
	require.True(t, protos.SameTransactionJSON([]byte(`{"TTxID":"tx","TChannel":"ch","TParameters":[""]}`), []byte(`{"TTxID":"tx","TChannel":"ch","TParameters":[""]}`)))
	require.False(t, protos.SameTransactionJSON([]byte(`{"TTxID":"tx","TChannel":"ch","TParameters":[""]}`), []byte(`{"TTxID":"tx","TChannel":"ch","TParameters":[null]}`)), "an empty parameter is not a null one")
}
