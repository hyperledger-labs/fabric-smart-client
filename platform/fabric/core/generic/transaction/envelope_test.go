/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction_test

import (
	"crypto/sha256"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction/mock"
)

func createValidEnvelope(tb testing.TB) *common.Envelope { //nolint:unparam
	tb.Helper()
	return createEnvelopeWithArgs(tb, [][]byte{[]byte("invoke"), []byte("arg1")})
}

// createEnvelopeWithArgs builds an envelope identical to createValidEnvelope but with
// a caller-controlled chaincode Args slice, so tests can craft the adversarial "zero
// arguments" case a malicious peer could send.
func createEnvelopeWithArgs(tb testing.TB, args [][]byte) *common.Envelope {
	tb.Helper()
	return &common.Envelope{Payload: buildEnvelopePayload(tb, func(m *envelopeMsgs) {
		m.cis.ChaincodeSpec.Input.Args = args
	})}
}

// envelopeMsgs holds the layers of an endorser transaction envelope payload.
type envelopeMsgs struct {
	chdr          *common.ChannelHeader
	shdr          *common.SignatureHeader
	cis           *peer.ChaincodeInvocationSpec
	cpp           *peer.ChaincodeProposalPayload
	ccAction      *peer.ChaincodeAction
	prp           *peer.ProposalResponsePayload
	actionPayload *peer.ChaincodeActionPayload
	tx            *peer.Transaction
	payload       *common.Payload
}

// buildEnvelopePayload marshals a valid envelope payload. Each layer is marshaled into
// the byte field of its parent unless mutate already set that field, so a test can
// replace, empty or drop a single layer. The response's proposal hash covers the
// envelope's proposal unless mutate set it.
func buildEnvelopePayload(tb testing.TB, mutate func(*envelopeMsgs)) []byte {
	tb.Helper()
	m := &envelopeMsgs{
		chdr: &common.ChannelHeader{Type: int32(common.HeaderType_ENDORSER_TRANSACTION), TxId: "txid", ChannelId: "channel"},
		shdr: &common.SignatureHeader{Creator: []byte("creator"), Nonce: []byte("nonce")},
		cis: &peer.ChaincodeInvocationSpec{ChaincodeSpec: &peer.ChaincodeSpec{
			ChaincodeId: &peer.ChaincodeID{Name: "mycc", Version: "1.0"},
			Input:       &peer.ChaincodeInput{Args: [][]byte{[]byte("invoke"), []byte("arg1")}},
		}},
		cpp:      &peer.ChaincodeProposalPayload{},
		ccAction: &peer.ChaincodeAction{Results: []byte("results")},
		prp:      &peer.ProposalResponsePayload{},
		actionPayload: &peer.ChaincodeActionPayload{Action: &peer.ChaincodeEndorsedAction{
			Endorsements: []*peer.Endorsement{{Endorser: []byte("endorser1"), Signature: []byte("sig1")}},
		}},
		tx:      &peer.Transaction{Actions: []*peer.TransactionAction{{}}},
		payload: &common.Payload{Header: &common.Header{}},
	}
	if mutate != nil {
		mutate(m)
	}
	fillBytes(tb, &m.cpp.Input, m.cis)
	fillBytes(tb, &m.actionPayload.ChaincodeProposalPayload, m.cpp)
	fillBytes(tb, &m.payload.Header.ChannelHeader, m.chdr)
	fillBytes(tb, &m.payload.Header.SignatureHeader, m.shdr)
	if m.prp.ProposalHash == nil {
		h := sha256.New()
		h.Write(m.payload.Header.ChannelHeader)
		h.Write(m.payload.Header.SignatureHeader)
		h.Write(m.actionPayload.ChaincodeProposalPayload)
		m.prp.ProposalHash = h.Sum(nil)
	}
	fillBytes(tb, &m.prp.Extension, m.ccAction)
	if m.actionPayload.Action != nil {
		fillBytes(tb, &m.actionPayload.Action.ProposalResponsePayload, m.prp)
	}
	if len(m.tx.Actions) != 0 {
		fillBytes(tb, &m.tx.Actions[0].Payload, m.actionPayload)
	}
	fillBytes(tb, &m.payload.Data, m.tx)
	return mustMarshal(tb, m.payload)
}

// fillBytes marshals msg into dst unless dst is already set.
func fillBytes(tb testing.TB, dst *[]byte, msg proto.Message) {
	tb.Helper()
	if *dst == nil {
		*dst = mustMarshal(tb, msg)
	}
}

func TestEnvelope(t *testing.T) {
	t.Parallel()
	env := createValidEnvelope(t)

	// Test NewEnvelopeFromEnv
	e, err := transaction.NewEnvelopeFromEnv(env)
	require.NoError(t, err)

	// Wait, UnpackEnvelopePayload was hit in other tests, but let's test invalid
	_, _, err = transaction.UnpackEnvelopePayload([]byte("invalid"))
	require.Error(t, err)

	_, err = transaction.GetChannelHeaderType([]byte("invalid"))
	require.Error(t, err)

	_, _, err = transaction.UnpackEnvelope(env)
	require.NoError(t, err)
	envStr := e.String()
	require.NotEmpty(t, envStr)

	require.NoError(t, err)
	require.NotNil(t, e)

	require.Equal(t, "txid", e.TxID())
	require.Equal(t, []byte("nonce"), e.Nonce())
	require.Equal(t, []byte("creator"), e.Creator())
	require.Equal(t, []byte("results"), e.Results())

	b, err := e.Bytes()
	require.NoError(t, err)
	require.NotEmpty(t, b)

	require.Equal(t, env, e.Envelope())

	s := e.String()
	require.NotEmpty(t, s)

	// Test UnpackEnvelopeFromBytes
	upe, ht, err := transaction.UnpackEnvelopeFromBytes(b)
	require.NoError(t, err)
	require.Equal(t, int32(common.HeaderType_ENDORSER_TRANSACTION), ht)
	require.NotNil(t, upe)

	require.Equal(t, "txid", upe.ID())
	require.Equal(t, "channel", upe.Channel())
	funcName, args := upe.FunctionAndParameters()
	require.Equal(t, "invoke", funcName)
	require.Equal(t, []string{"arg1"}, args)

	// Test GetChannelHeaderType
	htType, err := transaction.GetChannelHeaderType(b)
	require.NoError(t, err)
	require.Equal(t, common.HeaderType_ENDORSER_TRANSACTION, htType)

	// Test FromBytes
	e2 := transaction.NewEnvelope()
	err = e2.FromBytes(b)
	require.NoError(t, err)
	require.Equal(t, "txid", e2.TxID())
}

// TestUnpackEnvelopePayload_EmptyArgsReturnsError demonstrates that
// UnpackEnvelopePayload now rejects a zero-argument ChaincodeInvocationSpec with an
// error instead of panicking on the unchecked `cis.ChaincodeSpec.Input.Args[0]` index
// (envelope.go).
//
// This is directly responder-reachable: both platform/fabric/services/endorser/flow.go
// and platform/fabric/services/state/transaction.go's receiveTransactionView.Call read a
// raw []byte payload off an inbound P2P session and pass it straight into
// NewTransactionFromEnvelopeBytes / SetFromEnvelopeBytes. A remote peer sending an
// envelope whose ChaincodeInput has an empty Args slice must get a rejected
// transaction, not a crashed responder goroutine.
func TestUnpackEnvelopePayload_EmptyArgsReturnsError(t *testing.T) {
	t.Parallel()

	env := createEnvelopeWithArgs(t, [][]byte{})
	payloadBytes := env.Payload

	_, _, err := transaction.UnpackEnvelopePayload(payloadBytes)
	require.Error(t, err, "UnpackEnvelopePayload must reject a zero-argument ChaincodeInvocationSpec")
}

// TestTransaction_SetFromEnvelopeBytesReturnsErrorOnEmptyChaincodeArgs proves the fix is
// reachable through the exact production call chain used by the responder views:
// Transaction.SetFromEnvelopeBytes -> transaction.NewEnvelopeFromEnv/UnpackEnvelopePayload.
func TestTransaction_SetFromEnvelopeBytesReturnsErrorOnEmptyChaincodeArgs(t *testing.T) {
	t.Parallel()

	env := createEnvelopeWithArgs(t, [][]byte{})
	envBytes, err := proto.Marshal(env)
	require.NoError(t, err)

	tx := &transaction.Transaction{}
	err = tx.SetFromEnvelopeBytes(envBytes)
	require.Error(t, err, "SetFromEnvelopeBytes must reject a zero-argument ChaincodeInvocationSpec")
}

// TestUnpackEnvelopePayload_NilHeaderReturnsError demonstrates the fix for a
// nil-pointer-dereference bug found via fuzzing (FuzzUnpackEnvelopeFromBytes):
// UnpackEnvelopePayload (envelope.go) now checks payl.Header for nil before
// dereferencing it, instead of panicking with "invalid memory address or nil pointer
// dereference" the way protoutil.UnmarshalPayload/proto.Unmarshal's zero-value
// *common.Payload (with a nil Header) used to trigger.
//
// Unlike the Args[0] bug, this requires no crafted ChaincodeInvocationSpec at all:
// an envelope whose Payload is empty (or whose payload bytes simply omit the Header
// field) is enough. It is reachable through the exact same responder call chain:
// platform/fabric/services/endorser/flow.go and
// platform/fabric/services/state/transaction.go's receiveTransactionView.Call feed
// raw, unvalidated bytes from an inbound P2P session into
// NewTransactionFromEnvelopeBytes -> SetFromEnvelopeBytes -> UnpackEnvelopeFromBytes
// -> UnpackEnvelopePayload.
func TestUnpackEnvelopePayload_NilHeaderReturnsError(t *testing.T) {
	t.Parallel()

	_, _, err := transaction.UnpackEnvelopeFromBytes([]byte{})
	require.Error(t, err, "UnpackEnvelopeFromBytes must reject a marshaled payload with no Header")

	// Same fix reachable via the exact production call chain.
	tx := &transaction.Transaction{}
	err = tx.SetFromEnvelopeBytes([]byte{})
	require.Error(t, err, "SetFromEnvelopeBytes must reject an envelope payload with no Header")
}

// TestGetChannelHeaderType_NilHeaderReturnsError covers the same nil-Header guard for
// GetChannelHeaderType, whose payload-unmarshaling path mirrors UnpackEnvelopePayload's.
func TestGetChannelHeaderType_NilHeaderReturnsError(t *testing.T) {
	t.Parallel()

	envBytes, err := proto.Marshal(&common.Envelope{})
	require.NoError(t, err)

	_, err = transaction.GetChannelHeaderType(envBytes)
	require.Error(t, err, "GetChannelHeaderType must reject an envelope payload with no Header")
}

func TestEnvelope_Errors(t *testing.T) {
	t.Parallel()
	e := transaction.NewEnvelope()
	err := e.FromBytes([]byte("invalid bytes"))
	require.Error(t, err)

	_, err = transaction.NewEnvelopeFromEnv(&common.Envelope{Payload: []byte("invalid payload")})
	require.Error(t, err)

	_, _, err = transaction.UnpackEnvelopeFromBytes([]byte("invalid envelope bytes"))
	require.Error(t, err)

	_, err = transaction.GetChannelHeaderType([]byte("invalid bytes"))
	require.Error(t, err)
}

func TestUnpackEnvelopePayload_MalformedInput(t *testing.T) {
	t.Parallel()
	malformed := []byte("invalid")
	endorserTx := int32(common.HeaderType_ENDORSER_TRANSACTION)
	tests := []struct {
		name     string
		mutate   func(*envelopeMsgs)
		wantType int32
		wantErr  string
	}{
		{
			name:     "bad channel header",
			mutate:   func(m *envelopeMsgs) { m.payload.Header.ChannelHeader = malformed },
			wantType: -1,
			wantErr:  "failed to unmarshal channel header",
		},
		{
			name:     "non-endorser header type",
			mutate:   func(m *envelopeMsgs) { m.chdr.Type = int32(common.HeaderType_CONFIG) },
			wantType: int32(common.HeaderType_CONFIG),
			wantErr:  "only EndorserClient Transactions are supported, provided type 1",
		},
		{
			name:     "bad signature header",
			mutate:   func(m *envelopeMsgs) { m.payload.Header.SignatureHeader = malformed },
			wantType: endorserTx,
			wantErr:  "failed to unmarshal signature header",
		},
		{
			name:     "bad transaction",
			mutate:   func(m *envelopeMsgs) { m.payload.Data = malformed },
			wantType: endorserTx,
			wantErr:  "VSCC error: GetTransaction failed",
		},
		{
			name:     "no actions",
			mutate:   func(m *envelopeMsgs) { m.tx.Actions = nil },
			wantType: endorserTx,
			wantErr:  "VSCC error: transaction has no actions",
		},
		{
			name:     "bad action payload",
			mutate:   func(m *envelopeMsgs) { m.tx.Actions[0].Payload = malformed },
			wantType: endorserTx,
			wantErr:  "VSCC error: GetChaincodeActionPayload failed",
		},
		{
			name:     "bad chaincode proposal payload",
			mutate:   func(m *envelopeMsgs) { m.actionPayload.ChaincodeProposalPayload = malformed },
			wantType: endorserTx,
			wantErr:  "VSCC error: GetChaincodeProposalPayload failed",
		},
		{
			name:     "bad invocation spec",
			mutate:   func(m *envelopeMsgs) { m.cpp.Input = malformed },
			wantType: endorserTx,
			wantErr:  "VSCC error: UnmarshalChaincodeInvocationSpec failed",
		},
		{
			name:     "nil chaincode spec",
			mutate:   func(m *envelopeMsgs) { m.cis.ChaincodeSpec = nil },
			wantType: endorserTx,
			wantErr:  "chaincode invocation spec did not contain chaincode spec",
		},
		{
			name:     "nil input",
			mutate:   func(m *envelopeMsgs) { m.cis.ChaincodeSpec.Input = nil },
			wantType: endorserTx,
			wantErr:  "chaincode input did not contain any input",
		},
		{
			name:     "nil chaincode id",
			mutate:   func(m *envelopeMsgs) { m.cis.ChaincodeSpec.ChaincodeId = nil },
			wantType: endorserTx,
			wantErr:  "chaincode invocation spec did not contain chaincode id",
		},
		{
			name:     "nil action",
			mutate:   func(m *envelopeMsgs) { m.actionPayload.Action = nil },
			wantType: endorserTx,
			wantErr:  "VSCC error: chaincode action payload has no action",
		},
		{
			name:     "bad proposal response payload",
			mutate:   func(m *envelopeMsgs) { m.actionPayload.Action.ProposalResponsePayload = malformed },
			wantType: endorserTx,
			wantErr:  "failed to unmarshal proposal response payload",
		},
		{
			name:     "nil extension",
			mutate:   func(m *envelopeMsgs) { m.prp.Extension = []byte{} },
			wantType: endorserTx,
			wantErr:  "nil pRespPayload.Extension",
		},
		{
			name:     "bad chaincode action",
			mutate:   func(m *envelopeMsgs) { m.prp.Extension = malformed },
			wantType: endorserTx,
			wantErr:  "failed to unmarshal chaincode action",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			payload := buildEnvelopePayload(t, tc.mutate)

			var (
				upe *transaction.UnpackedEnvelope
				ht  int32
				err error
			)
			require.NotPanics(t, func() { upe, ht, err = transaction.UnpackEnvelopePayload(payload) })
			require.ErrorContains(t, err, tc.wantErr)
			require.Nil(t, upe)
			require.Equal(t, tc.wantType, ht)
		})
	}
}

func TestGetChannelHeaderType_MalformedInput(t *testing.T) {
	t.Parallel()
	malformed := []byte("invalid")

	_, err := transaction.GetChannelHeaderType(mustMarshal(t, &common.Envelope{Payload: malformed}))
	require.ErrorContains(t, err, "failed to unmarshal payload")

	payload := buildEnvelopePayload(t, func(m *envelopeMsgs) { m.payload.Header.ChannelHeader = malformed })
	_, err = transaction.GetChannelHeaderType(mustMarshal(t, &common.Envelope{Payload: payload}))
	require.ErrorContains(t, err, "failed to unmarshal channel header")
}

func TestEnvelope_FromBytesUndecodablePayload(t *testing.T) {
	t.Parallel()
	e := transaction.NewEnvelope()
	err := e.FromBytes(mustMarshal(t, &common.Envelope{Payload: []byte("invalid")}))
	require.ErrorContains(t, err, "failed to unmarshal payload")
}

// TestTransaction_SetFromEnvelopeBytesRejectsUnendorsedProposal checks that an envelope
// whose proposal is not the one its endorsements sign is rejected, so the function and
// arguments read from it cannot be rewritten.
func TestTransaction_SetFromEnvelopeBytesRejectsUnendorsedProposal(t *testing.T) {
	t.Parallel()

	upe, _, err := transaction.UnpackEnvelope(createValidEnvelope(t))
	require.NoError(t, err)
	endorsed := upe.EndorsedProposalHash
	require.Equal(t, upe.ProposalHash, endorsed)

	for _, tc := range []struct {
		name   string
		mutate func(*envelopeMsgs)
	}{
		{name: "rewritten arguments", mutate: func(m *envelopeMsgs) {
			m.cis.ChaincodeSpec.Input.Args = [][]byte{[]byte("invoke"), []byte("other-arg")}
			m.prp.ProposalHash = endorsed
		}},
		{name: "rewritten tx id", mutate: func(m *envelopeMsgs) {
			m.chdr.TxId = "other-txid"
			m.prp.ProposalHash = endorsed
		}},
		{name: "wrong proposal hash", mutate: func(m *envelopeMsgs) { m.prp.ProposalHash = []byte("other") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			channelProvider := &mock.ChannelProvider{}
			channelProvider.ChannelReturns(&mock.Channel{}, nil)
			tx, err := transaction.NewEndorserTransactionFactory("network", channelProvider, &mock.SignerService{}).
				NewTransaction(t.Context(), "channel", nil, nil, "", nil)
			require.NoError(t, err)

			raw := mustMarshal(t, &common.Envelope{Payload: buildEnvelopePayload(t, tc.mutate)})
			require.ErrorContains(t, tx.SetFromEnvelopeBytes(raw), "envelope proposal hash does not match the endorsed proposal hash")
		})
	}
}
