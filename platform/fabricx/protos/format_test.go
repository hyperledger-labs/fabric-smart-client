/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protos_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/hyperledger/fabric-x-common/api/applicationpb"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	fprotos "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/protos"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/transaction/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/protos"
)

// fabricxNode is a Fabric-x transaction manager wired to mocks that let its
// transactions endorse themselves with an X.509 identity.
type fabricxNode struct {
	manager  *transaction.Manager
	channel  *mock.Channel
	identity []byte
}

func newFabricxNode(t *testing.T) *fabricxNode {
	t.Helper()
	n := &fabricxNode{channel: &mock.Channel{}, identity: x509Identity(t, "Org1MSP")}
	rawTx, err := proto.Marshal(&applicationpb.Tx{Namespaces: []*applicationpb.TxNamespace{{
		NsId:        "ns1",
		BlindWrites: []*applicationpb.Write{{Key: []byte("k"), Value: []byte("v")}},
	}}})
	require.NoError(t, err)
	rwset := &mock.RWSet{}
	rwset.BytesReturns(rawTx, nil)
	vault := &mock.Vault{}
	vault.NewRWSetReturns(rwset, nil)
	vault.NewRWSetFromBytesReturns(rwset, nil)
	n.channel.VaultReturns(vault)
	n.channel.MetadataServiceReturns(&mock.MetadataService{})
	signer := &mock.Signer{}
	signer.SignReturns([]byte("sig"), nil)
	sigService := &mock.SignerService{}
	sigService.GetSignerReturns(signer, nil)
	fns := &mock.FabricNetworkService{}
	fns.NameReturns("network")
	fns.ChannelReturns(n.channel, nil)
	fns.SignerServiceReturns(sigService)

	n.manager = transaction.NewManager()
	n.manager.AddTransactionFactory(driver.EndorserTransaction, transaction.NewFactory(fns))
	return n
}

func x509Identity(t *testing.T, mspID string) []byte {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: mspID},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	require.NoError(t, err)
	raw, err := proto.Marshal(&msp.SerializedIdentity{Mspid: mspID, IdBytes: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})})
	require.NoError(t, err)
	return raw
}

// endorsedTx returns the JSON of a transaction endorsed on n, with key as its channel key.
func (n *fabricxNode) endorsedTx(t *testing.T, key *driver.ChannelKey) (*transaction.Transaction, []byte) {
	t.Helper()
	n.channel.ChannelKeyReturns(key, nil)
	tx, err := n.manager.NewTransaction(t.Context(), driver.EndorserTransaction, n.identity, []byte("nonce"), "txid", "channel", nil)
	require.NoError(t, err)
	sent := tx.(*transaction.Transaction)
	sent.SetProposal("mycc", "1.0", "invoke", "a1", "a2")
	require.NoError(t, sent.SetRWSet())
	require.NoError(t, sent.EndorseWithIdentity(n.identity))
	sent.TTransient["k"] = []byte("v")
	raw, err := sent.Bytes()
	require.NoError(t, err)
	return sent, raw
}

func TestRoundTripFabricx(t *testing.T) {
	t.Parallel()
	key := &driver.ChannelKey{Platform: protos.Platform, Channel: "channel", GenesisHash: []byte("genesis")}
	sent, raw := newFabricxNode(t).endorsedTx(t, key)

	env, err := fprotos.EnvelopeFromJSON(raw)
	require.NoError(t, err)
	var msg fprotos.TransactionEnvelope
	require.NoError(t, proto.Unmarshal(env, &msg))
	require.Equal(t, protos.Platform, msg.GetPlatform())
	var payload protos.Transaction
	require.NoError(t, proto.Unmarshal(msg.GetPayload(), &payload))
	require.Equal(t, "txid", payload.GetBase().GetTxId())

	back, err := fprotos.JSONFor(env, protos.Platform)
	require.NoError(t, err)
	decoded, err := newFabricxNode(t).manager.NewTransactionFromBytes(t.Context(), "channel", back)
	require.NoError(t, err)
	got := decoded.(*transaction.Transaction)

	require.Equal(t, sent.ID(), got.ID())
	require.Equal(t, sent.TFunction, got.TFunction)
	require.Equal(t, sent.TParameters, got.TParameters)
	require.Equal(t, sent.TTransient, got.TTransient)
	require.Equal(t, sent.TSignedProposal.ProposalBytes, got.TSignedProposal.ProposalBytes)
	require.Equal(t, sent.TSignedProposal.Signature, got.TSignedProposal.Signature)
	require.Len(t, got.TProposalResponses, len(sent.TProposalResponses))
	for i := range sent.TProposalResponses {
		require.Equal(t, sent.TProposalResponses[i].Payload, got.TProposalResponses[i].Payload)
		require.True(t, proto.Equal(sent.TProposalResponses[i].Endorsement, got.TProposalResponses[i].Endorsement))
	}

	var flat transaction.Transaction
	require.NoError(t, json.Unmarshal(back, &flat))
	require.Equal(t, key, flat.Key)
	require.NotEqual(t, raw, back)
	require.True(t, fprotos.SameTransactionJSON(raw, back), "the JSON reads back as the same transaction")
	sent.TParameters[0] = []byte("other")
	other, err := sent.Bytes()
	require.NoError(t, err)
	require.False(t, fprotos.SameTransactionJSON(other, back))
}

// A sender whose channel key cannot be read yet sends an envelope without a genesis hash; the
// store, which cannot tell the platform of such JSON, keeps it as JSON.
func TestKeylessFabricx(t *testing.T) {
	t.Parallel()
	_, raw := newFabricxNode(t).endorsedTx(t, nil)

	_, err := fprotos.EnvelopeFromJSON(raw)
	require.ErrorIs(t, err, fprotos.ErrNoChannelKey)

	env, err := fprotos.EnvelopeFromJSONFor(raw, protos.Platform)
	require.NoError(t, err)
	var msg fprotos.TransactionEnvelope
	require.NoError(t, proto.Unmarshal(env, &msg))
	require.Empty(t, msg.GetChannelGenesisHash())
	back, err := fprotos.JSONFor(env, protos.Platform)
	require.NoError(t, err)
	_, err = newFabricxNode(t).manager.NewTransactionFromBytes(t.Context(), "channel", back)
	require.NoError(t, err)
}

func TestFabricxFormat(t *testing.T) {
	t.Parallel()
	f, ok := fprotos.TransactionFormatOf(protos.Platform)
	require.True(t, ok)
	require.True(t, f.SkipTxIDCheck)
}

func TestFabricxRejects(t *testing.T) {
	t.Parallel()
	key := &driver.ChannelKey{Platform: protos.Platform, Channel: "channel", GenesisHash: []byte("genesis")}
	_, raw := newFabricxNode(t).endorsedTx(t, key)
	env, err := fprotos.EnvelopeFromJSON(raw)
	require.NoError(t, err)
	valid := &fprotos.TransactionEnvelope{}
	require.NoError(t, proto.Unmarshal(env, valid))

	for name, tc := range map[string]struct {
		edit func(e *fprotos.TransactionEnvelope)
		want string
	}{
		"channel mismatch": {func(e *fprotos.TransactionEnvelope) { e.Channel = "other" }, "[other]"},
		"no base":          {func(e *fprotos.TransactionEnvelope) { e.Payload = nil }, "no transaction"},
		"bad payload":      {func(e *fprotos.TransactionEnvelope) { e.Payload = []byte{0xff} }, "unmarshal transaction"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := proto.CloneOf(valid)
			tc.edit(e)
			bad, err := proto.Marshal(e)
			require.NoError(t, err)
			_, err = fprotos.JSONFromEnvelope(bad)
			require.ErrorContains(t, err, tc.want)
		})
	}

	for name, tc := range map[string]struct {
		raw  string
		want string
	}{
		"unknown field":        {`{"TTxID":"tx","TChannel":"channel","Extra":1,"Key":{"platform":"fabricx","channel":"channel"}}`, "unknown field"},
		"key channel mismatch": {`{"TTxID":"tx","TChannel":"channel","Key":{"platform":"fabricx","channel":"other"}}`, "[other]"},
		"no transaction ID":    {`{"TChannel":"channel","Key":{"platform":"fabricx","channel":"channel"}}`, "no transaction ID"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := fprotos.EnvelopeFromJSON([]byte(tc.raw))
			require.ErrorContains(t, err, tc.want)
		})
	}
}
