/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rwset

import (
	"testing"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	rwsetfake "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/rwset/fake"
	rwsetmock "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/rwset/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	qsmock "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice/mock"
	txmock "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/transaction/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/vault"
)

const (
	testNetwork = "network"
	testChannel = "mychannel"
	testTxID    = "tx1"
)

var errSentinel = errors.New("sentinel")

// newTestVault returns a real fabricx vault whose query service pins a namespace
// version for ns1 and ns2, so RWSets over those namespaces can be serialized.
func newTestVault() *vault.Vault {
	qs := &qsmock.QueryService{}
	qs.GetStatesReturns(map[cdriver.Namespace]map[cdriver.PKey]cdriver.VaultValue{
		"_meta": {
			"ns1": {Version: vault.MarshalVersion(1)},
			"ns2": {Version: vault.MarshalVersion(1)},
		},
	}, nil)
	return vault.NewVault(qs, nil)
}

// buildRWSetBytes serializes an RWSet that reads ns1/k1 and ns2/readKey and writes
// ns1/k2=v2.
func buildRWSetBytes(t *testing.T, v *vault.Vault, readKey string) []byte {
	t.Helper()
	rws, err := v.NewRWSet(t.Context(), testTxID)
	require.NoError(t, err)
	require.NoError(t, rws.AddReadAt("ns1", "k1", vault.MarshalVersion(3)))
	require.NoError(t, rws.AddReadAt("ns2", readKey, vault.MarshalVersion(4)))
	require.NoError(t, rws.SetState("ns1", "k2", []byte("v2")))
	raw, err := rws.Bytes()
	require.NoError(t, err)
	return raw
}

func buildEnvelope(txID string, data []byte) []byte {
	return buildEnvelopeFor(cb.HeaderType_MESSAGE, testChannel, txID, data)
}

func buildEnvelopeFor(headerType cb.HeaderType, channel, txID string, data []byte) []byte {
	chdr := protoutil.MakeChannelHeader(headerType, 0, channel, 0)
	chdr.TxId = txID
	payl := &cb.Payload{
		Header: protoutil.MakePayloadHeader(chdr, &cb.SignatureHeader{}),
		Data:   data,
	}
	return protoutil.MarshalOrPanic(&cb.Envelope{Payload: protoutil.MarshalOrPanic(payl)})
}

// requireRWSetContent checks the namespaces, reads and writes of buildRWSetBytes.
func requireRWSetContent(t *testing.T, rws driver.RWSet, readKey string) {
	t.Helper()
	require.ElementsMatch(t, []cdriver.Namespace{"ns1", "ns2"}, rws.Namespaces())

	require.Equal(t, 1, rws.NumReads("ns1"))
	k, err := rws.GetReadKeyAt("ns1", 0)
	require.NoError(t, err)
	require.Equal(t, "k1", k)
	require.Equal(t, 1, rws.NumReads("ns2"))
	k, err = rws.GetReadKeyAt("ns2", 0)
	require.NoError(t, err)
	require.Equal(t, readKey, k)

	require.Equal(t, 1, rws.NumWrites("ns1"))
	k, val, err := rws.GetWriteAt("ns1", 0)
	require.NoError(t, err)
	require.Equal(t, "k2", k)
	require.Equal(t, []byte("v2"), val)
	require.Zero(t, rws.NumWrites("ns2"))
}

func requireProcessedTransaction(t *testing.T, pt driver.ProcessTransaction, function string) {
	t.Helper()
	require.Equal(t, testNetwork, pt.Network())
	require.Equal(t, testChannel, pt.Channel())
	require.Equal(t, testTxID, pt.ID())
	fn, params := pt.FunctionAndParameters()
	require.Equal(t, function, fn)
	require.Empty(t, params)
}

// malformedEnvelopes are envelopes for testTxID on testChannel that fail to parse at
// different depths or do not match the requested transaction.
type malformedCase struct {
	raw     []byte
	errText string
}

func malformedEnvelopes(t *testing.T, v *vault.Vault) map[string]malformedCase {
	t.Helper()
	rwset := buildRWSetBytes(t, v, "k")
	valid := buildEnvelope(testTxID, rwset)
	return map[string]malformedCase{
		"unsupported header type": {
			raw:     buildEnvelopeFor(cb.HeaderType_ENDORSER_TRANSACTION, testChannel, testTxID, rwset),
			errText: "unsupported header type ENDORSER_TRANSACTION, expected MESSAGE",
		},
		"txID mismatch": {
			raw:     buildEnvelope("other", rwset),
			errText: "txID mismatch in channel header, expected=tx1, actual=other",
		},
		"channel mismatch": {
			raw:     buildEnvelopeFor(cb.HeaderType_MESSAGE, "other", testTxID, rwset),
			errText: "channel mismatch in channel header, expected=mychannel, actual=other",
		},
		"truncated envelope": {raw: valid[:len(valid)/2], errText: "Error getting tx from block"},
		"payload is not a payload": {
			raw:     protoutil.MarshalOrPanic(&cb.Envelope{Payload: []byte("not-a-payload")}),
			errText: "unmarshal payload failed",
		},
		"payload without header": {
			raw:     protoutil.MarshalOrPanic(&cb.Envelope{Payload: protoutil.MarshalOrPanic(&cb.Payload{Data: []byte("x")})}),
			errText: "envelope must have a Header",
		},
		"payload carries another message": {
			// A channel header shares no field numbers with a payload, so it decodes as
			// a payload without header.
			raw: protoutil.MarshalOrPanic(&cb.Envelope{Payload: protoutil.MarshalOrPanic(
				protoutil.MakeChannelHeader(cb.HeaderType_MESSAGE, 0, testChannel, 0),
			)}),
			errText: "envelope must have a Header",
		},
		"channel header is not a channel header": {
			raw: protoutil.MarshalOrPanic(&cb.Envelope{Payload: protoutil.MarshalOrPanic(&cb.Payload{
				Header: &cb.Header{ChannelHeader: []byte("not-a-channel-header")},
			})}),
			errText: "unmarshal channel header failed",
		},
	}
}

func TestNewLoader(t *testing.T) {
	t.Parallel()
	envSvc := &rwsetfake.EnvelopeService{}
	txSvc := &rwsetfake.EndorserTransactionService{}
	tm := &rwsetmock.TransactionManager{}
	v := &rwsetmock.RWSetInspector{}

	l, ok := NewLoader(testNetwork, testChannel, envSvc, txSvc, tm, v).(*Loader)
	require.True(t, ok)
	require.Equal(t, testNetwork, l.Network)
	require.Equal(t, testChannel, l.Channel)
	require.Same(t, envSvc, l.EnvelopeService)
	require.Same(t, txSvc, l.TransactionService)
	require.Same(t, tm, l.TransactionManager)
	require.Same(t, v, l.Vault)
}

func TestAddHandlerProvider(t *testing.T) {
	t.Parallel()
	l := NewLoader(testNetwork, testChannel, nil, nil, nil, nil)
	provider := func(string, string, driver.RWSetInspector) driver.RWSetPayloadHandler {
		require.Fail(t, "provider must not be invoked")
		return nil
	}

	// Registration is a no-op, so repeating it succeeds as well.
	require.NoError(t, l.AddHandlerProvider(cb.HeaderType_ENDORSER_TRANSACTION, provider))
	require.NoError(t, l.AddHandlerProvider(cb.HeaderType_ENDORSER_TRANSACTION, provider))
}

func TestGetRWSetFromEvn(t *testing.T) {
	t.Parallel()

	newLoader := func(envSvc driver.EnvelopeService, v driver.RWSetInspector) driver.RWSetLoader {
		return NewLoader(testNetwork, testChannel, envSvc, nil, nil, v)
	}

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		v := newTestVault()
		env := buildEnvelope(testTxID, buildRWSetBytes(t, v, "k"))
		rws, pt, err := newLoader(&rwsetfake.EnvelopeService{ExistsValue: true, Envelope: env}, v).
			GetRWSetFromEvn(t.Context(), testTxID)
		require.NoError(t, err)
		requireRWSetContent(t, rws, "k")
		requireProcessedTransaction(t, pt, "")
	})

	t.Run("init function detected", func(t *testing.T) {
		t.Parallel()
		v := newTestVault()
		env := buildEnvelope(testTxID, buildRWSetBytes(t, v, "ns.initialized"))
		_, pt, err := newLoader(&rwsetfake.EnvelopeService{ExistsValue: true, Envelope: env}, v).
			GetRWSetFromEvn(t.Context(), testTxID)
		require.NoError(t, err)
		requireProcessedTransaction(t, pt, "init")
	})

	t.Run("envelope missing", func(t *testing.T) {
		t.Parallel()
		_, _, err := newLoader(&rwsetfake.EnvelopeService{}, nil).GetRWSetFromEvn(t.Context(), testTxID)
		require.ErrorContains(t, err, "envelope does not exists for [txID=tx1]")
	})

	t.Run("load error", func(t *testing.T) {
		t.Parallel()
		_, _, err := newLoader(&rwsetfake.EnvelopeService{ExistsValue: true, LoadErr: errSentinel}, nil).
			GetRWSetFromEvn(t.Context(), testTxID)
		require.ErrorIs(t, err, errSentinel)
		require.ErrorContains(t, err, "load envelope [txID=tx1]")
	})

	t.Run("malformed envelope", func(t *testing.T) {
		t.Parallel()
		for name, tc := range malformedEnvelopes(t, newTestVault()) {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				_, _, err := newLoader(&rwsetfake.EnvelopeService{ExistsValue: true, Envelope: tc.raw}, nil).
					GetRWSetFromEvn(t.Context(), testTxID)
				require.ErrorContains(t, err, "unmarshal payload and channel header")
				require.ErrorContains(t, err, tc.errText)
			})
		}
	})

	t.Run("payload data is not an rwset", func(t *testing.T) {
		t.Parallel()
		env := buildEnvelope(testTxID, []byte("not-an-rwset"))
		_, _, err := newLoader(&rwsetfake.EnvelopeService{ExistsValue: true, Envelope: env}, newTestVault()).
			GetRWSetFromEvn(t.Context(), testTxID)
		require.ErrorContains(t, err, "create new rws for [txID=tx1]")
		require.ErrorContains(t, err, "failed to unmarshal rwset")
	})

	t.Run("payload data carrying another message is rejected", func(t *testing.T) {
		t.Parallel()
		data := protoutil.MarshalOrPanic(protoutil.MakeChannelHeader(cb.HeaderType_MESSAGE, 0, "x", 0))
		env := buildEnvelope(testTxID, data)
		_, _, err := newLoader(&rwsetfake.EnvelopeService{ExistsValue: true, Envelope: env}, newTestVault()).
			GetRWSetFromEvn(t.Context(), testTxID)
		require.ErrorContains(t, err, "unknown fields in")
	})

	t.Run("vault error is wrapped", func(t *testing.T) {
		t.Parallel()
		inspector := &rwsetmock.RWSetInspector{}
		inspector.NewRWSetFromBytesReturns(nil, errSentinel)
		env := buildEnvelope(testTxID, []byte("data"))
		_, _, err := newLoader(&rwsetfake.EnvelopeService{ExistsValue: true, Envelope: env}, inspector).
			GetRWSetFromEvn(t.Context(), testTxID)
		require.ErrorIs(t, err, errSentinel)
		_, txID, data := inspector.NewRWSetFromBytesArgsForCall(0)
		require.Equal(t, testTxID, txID)
		require.Equal(t, []byte("data"), data)
	})
}

func TestGetRWSetFromETx(t *testing.T) {
	t.Parallel()

	newLoader := func(txSvc driver.EndorserTransactionService, tm driver.TransactionManager) driver.RWSetLoader {
		return NewLoader(testNetwork, testChannel, nil, txSvc, tm, nil)
	}

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		v := newTestVault()
		expected, err := v.NewRWSetFromBytes(t.Context(), testTxID, buildRWSetBytes(t, v, "k"))
		require.NoError(t, err)
		tx := &rwsetfake.Transaction{RWSetValue: expected}
		tm := &rwsetmock.TransactionManager{}
		tm.NewTransactionFromBytesReturns(tx, nil)

		rws, pt, err := newLoader(&rwsetfake.EndorserTransactionService{ExistsValue: true, Transaction: []byte("raw")}, tm).
			GetRWSetFromETx(t.Context(), testTxID)
		require.NoError(t, err)
		require.Same(t, expected, rws)
		requireRWSetContent(t, rws, "k")
		require.Same(t, tx, pt)

		_, channel, raw := tm.NewTransactionFromBytesArgsForCall(0)
		require.Equal(t, testChannel, channel)
		require.Equal(t, []byte("raw"), raw)
	})

	t.Run("transaction missing", func(t *testing.T) {
		t.Parallel()
		_, _, err := newLoader(&rwsetfake.EndorserTransactionService{}, nil).GetRWSetFromETx(t.Context(), testTxID)
		require.ErrorContains(t, err, "transaction does not exists for [txID=tx1]")
	})

	t.Run("load error", func(t *testing.T) {
		t.Parallel()
		_, _, err := newLoader(&rwsetfake.EndorserTransactionService{ExistsValue: true, LoadErr: errSentinel}, nil).
			GetRWSetFromETx(t.Context(), testTxID)
		require.ErrorIs(t, err, errSentinel)
		require.ErrorContains(t, err, "cannot load etx [txID=tx1]")
	})

	t.Run("malformed transaction", func(t *testing.T) {
		t.Parallel()
		tm := &rwsetmock.TransactionManager{}
		tm.NewTransactionFromBytesReturns(nil, errSentinel)
		_, _, err := newLoader(&rwsetfake.EndorserTransactionService{ExistsValue: true, Transaction: []byte("trunc")}, tm).
			GetRWSetFromETx(t.Context(), testTxID)
		require.ErrorIs(t, err, errSentinel)
		require.ErrorContains(t, err, "new transaction from bytes")
	})

	t.Run("get rwset error", func(t *testing.T) {
		t.Parallel()
		tm := &rwsetmock.TransactionManager{}
		tm.NewTransactionFromBytesReturns(&rwsetfake.Transaction{RWSetErr: errSentinel}, nil)
		_, _, err := newLoader(&rwsetfake.EndorserTransactionService{ExistsValue: true, Transaction: []byte("raw")}, tm).
			GetRWSetFromETx(t.Context(), testTxID)
		require.ErrorIs(t, err, errSentinel)
		require.ErrorContains(t, err, "get rwset")
	})
}

func TestGetInspectingRWSetFromEvn(t *testing.T) {
	t.Parallel()

	newLoader := func(v driver.RWSetInspector) driver.RWSetLoader {
		return NewLoader(testNetwork, testChannel, nil, nil, nil, v)
	}

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		v := newTestVault()
		env := buildEnvelope(testTxID, buildRWSetBytes(t, v, "ns.initialized"))
		rws, pt, err := newLoader(v).GetInspectingRWSetFromEvn(t.Context(), testTxID, env)
		require.NoError(t, err)
		requireRWSetContent(t, rws, "ns.initialized")
		requireProcessedTransaction(t, pt, "init")
	})

	t.Run("malformed envelope", func(t *testing.T) {
		t.Parallel()
		for name, tc := range malformedEnvelopes(t, newTestVault()) {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				_, _, err := newLoader(nil).GetInspectingRWSetFromEvn(t.Context(), testTxID, tc.raw)
				require.ErrorContains(t, err, "cannot unmarshal envelope [txID=tx1]")
				require.ErrorContains(t, err, tc.errText)
			})
		}
	})

	t.Run("payload data is not an rwset", func(t *testing.T) {
		t.Parallel()
		env := buildEnvelope(testTxID, []byte("not-an-rwset"))
		_, _, err := newLoader(newTestVault()).GetInspectingRWSetFromEvn(t.Context(), testTxID, env)
		require.ErrorContains(t, err, "cannot inspect rwset for [txID=tx1]")
		require.ErrorContains(t, err, "failed to unmarshal rwset for inspection")
	})

	t.Run("vault error is wrapped", func(t *testing.T) {
		t.Parallel()
		inspector := &rwsetmock.RWSetInspector{}
		inspector.InspectRWSetReturns(nil, errSentinel)
		env := buildEnvelope(testTxID, []byte("data"))
		_, _, err := newLoader(inspector).GetInspectingRWSetFromEvn(t.Context(), testTxID, env)
		require.ErrorIs(t, err, errSentinel)
		_, data, nss := inspector.InspectRWSetArgsForCall(0)
		require.Equal(t, []byte("data"), data)
		require.Empty(t, nss)
	})
}

func TestAnyKeyContains(t *testing.T) {
	t.Parallel()
	v := newTestVault()

	t.Run("match in later namespace", func(t *testing.T) {
		t.Parallel()
		// ns1 only reads k1; the match is ns2's read key.
		rws, err := v.InspectRWSet(t.Context(), buildRWSetBytes(t, v, "ns.initialized"))
		require.NoError(t, err)
		require.True(t, anyKeyContains(rws, "initialized"))
	})

	t.Run("no match", func(t *testing.T) {
		t.Parallel()
		rws, err := v.InspectRWSet(t.Context(), buildRWSetBytes(t, v, "k"))
		require.NoError(t, err)
		require.False(t, anyKeyContains(rws, "initialized"))
		// Written keys are not considered.
		require.False(t, anyKeyContains(rws, "k2"))
	})

	t.Run("read key errors are skipped", func(t *testing.T) {
		t.Parallel()
		rws := &txmock.RWSet{}
		rws.NamespacesReturns([]cdriver.Namespace{"ns1"})
		rws.NumReadsReturns(2)
		rws.GetReadKeyAtReturnsOnCall(0, "initialized", errSentinel)
		rws.GetReadKeyAtReturnsOnCall(1, "k", nil)
		require.False(t, anyKeyContains(rws, "initialized"))
		require.Equal(t, 2, rws.GetReadKeyAtCallCount())
	})

	t.Run("empty rwset", func(t *testing.T) {
		t.Parallel()
		rws, err := v.NewRWSet(t.Context(), testTxID)
		require.NoError(t, err)
		require.False(t, anyKeyContains(rws, ""))
	})
}
