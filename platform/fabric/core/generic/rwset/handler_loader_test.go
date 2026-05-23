/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rwset

import (
	"context"
	stderrors "errors"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/require"

	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

func TestEndorserTransactionHandlerLoad(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		results := []byte("rwset-bytes")
		_, payl, chdr, _, _, _ := buildTestEnvelope(t, cb.HeaderType_ENDORSER_TRANSACTION, results)
		expectedRWS := &fakeRWSet{namespaces: []cdriver.Namespace{"ns1"}}
		inspector := &fakeInspector{
			newFunc: func(_ context.Context, txID cdriver.TxID, rwset []byte) (fdriver.RWSet, error) {
				require.Equal(t, cdriver.TxID(chdr.TxId), txID)
				require.Equal(t, results, rwset)
				return expectedRWS, nil
			},
		}

		h := NewEndorserTransactionHandler("test-network", "mychannel", inspector)
		rws, tx, err := h.Load(payl, chdr)
		require.NoError(t, err)
		require.Same(t, expectedRWS, rws)
		require.Equal(t, chdr.TxId, tx.ID())
		require.Equal(t, "test-network", tx.Network())
		require.Equal(t, "mychannel", tx.Channel())
	})

	t.Run("unpack error", func(t *testing.T) {
		t.Parallel()
		_, payl, chdr, _, _, _ := buildTestEnvelope(t, cb.HeaderType_CONFIG, []byte("rwset"))
		h := NewEndorserTransactionHandler("test-network", "mychannel", &fakeInspector{})
		_, _, err := h.Load(payl, chdr)
		require.Error(t, err)
		require.ErrorContains(t, err, "failed unpacking envelope")
	})

	t.Run("rwset inspector error", func(t *testing.T) {
		t.Parallel()
		_, payl, chdr, _, _, _ := buildTestEnvelope(t, cb.HeaderType_ENDORSER_TRANSACTION, []byte("rwset"))
		inspector := &fakeInspector{
			newFunc: func(_ context.Context, _ cdriver.TxID, _ []byte) (fdriver.RWSet, error) {
				return nil, stderrors.New("new-rwset-failed")
			},
		}
		h := NewEndorserTransactionHandler("test-network", "mychannel", inspector)
		_, _, err := h.Load(payl, chdr)
		require.ErrorContains(t, err, "new-rwset-failed")
	})
}

func TestEndorserTransactionReaderRead(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		results := buildValidRWSetBytes(t)
		_, payl, chdr, _, _, _ := buildTestEnvelope(t, cb.HeaderType_ENDORSER_TRANSACTION, results)

		reader := NewEndorserTransactionReader("test-network")
		rws, err := reader.Read(payl, chdr)
		require.NoError(t, err)

		_, in := rws.ReadSet.Get("ns1", "k1")
		require.True(t, in)
		require.Equal(t, []byte("v2"), rws.WriteSet.Get("ns1", "k2"))
		require.Equal(t, []byte("mv1"), rws.MetaWriteSet.Get("ns1", "k3")["m1"])
	})

	t.Run("populate error", func(t *testing.T) {
		t.Parallel()
		_, payl, chdr, _, _, _ := buildTestEnvelope(t, cb.HeaderType_ENDORSER_TRANSACTION, []byte("bad-rwset"))
		reader := NewEndorserTransactionReader("test-network")
		_, err := reader.Read(payl, chdr)
		require.Error(t, err)
		require.ErrorContains(t, err, "provided invalid read-write set bytes")
	})
}

func TestLoaderAddHandlerProvider(t *testing.T) {
	t.Parallel()

	loader := NewLoader("network", "channel", nil, nil, nil, nil)
	provider := func(_, _ string, _ fdriver.RWSetInspector) fdriver.RWSetPayloadHandler {
		return &fakeRWSetHandler{
			loadFn: func(_ *cb.Payload, _ *cb.ChannelHeader) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
				return nil, nil, nil
			},
		}
	}

	err := loader.AddHandlerProvider(cb.HeaderType_ENDORSER_TRANSACTION, provider)
	require.NoError(t, err)

	err = loader.AddHandlerProvider(cb.HeaderType_ENDORSER_TRANSACTION, provider)
	require.Error(t, err)
	require.ErrorContains(t, err, "already defined")
}

func TestLoaderGetRWSetFromEvn(t *testing.T) {
	t.Parallel()

	t.Run("envelope missing", func(t *testing.T) {
		t.Parallel()
		loader := NewLoader(
			"network",
			"channel",
			&fakeEnvelopeService{existsFn: func(_ context.Context, _ string) bool { return false }},
			nil,
			nil,
			nil,
		)
		_, _, err := loader.GetRWSetFromEvn(t.Context(), "tx1")
		require.Error(t, err)
		require.ErrorContains(t, err, "envelope does not exist")
	})

	t.Run("load envelope error", func(t *testing.T) {
		t.Parallel()
		loader := NewLoader(
			"network",
			"channel",
			&fakeEnvelopeService{
				existsFn: func(_ context.Context, _ string) bool { return true },
				loadFn:   func(_ context.Context, _ string) ([]byte, error) { return nil, stderrors.New("load-failed") },
			},
			nil,
			nil,
			nil,
		)
		_, _, err := loader.GetRWSetFromEvn(t.Context(), "tx1")
		require.Error(t, err)
		require.ErrorContains(t, err, "cannot load envelope")
	})

	t.Run("invalid envelope bytes", func(t *testing.T) {
		t.Parallel()
		loader := NewLoader(
			"network",
			"channel",
			&fakeEnvelopeService{
				existsFn: func(_ context.Context, _ string) bool { return true },
				loadFn:   func(_ context.Context, _ string) ([]byte, error) { return []byte("bad-envelope"), nil },
			},
			nil,
			nil,
			nil,
		)
		_, _, err := loader.GetRWSetFromEvn(t.Context(), "tx1")
		require.Error(t, err)
		require.ErrorContains(t, err, "failed unmarshalling envelope")
	})

	t.Run("unsupported header type", func(t *testing.T) {
		t.Parallel()
		env, _, chdr, _, _, _ := buildTestEnvelope(t, cb.HeaderType_CONFIG, []byte("rwset"))
		loader := NewLoader(
			"network",
			"channel",
			&fakeEnvelopeService{
				existsFn: func(_ context.Context, _ string) bool { return true },
				loadFn:   func(_ context.Context, _ string) ([]byte, error) { return mustMarshalProto(t, env), nil },
			},
			nil,
			nil,
			nil,
		)
		_, _, err := loader.GetRWSetFromEvn(t.Context(), cdriver.TxID(chdr.TxId))
		require.Error(t, err)
		require.ErrorContains(t, err, "header type not supported")
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		env, _, chdr, _, _, _ := buildTestEnvelope(t, cb.HeaderType_ENDORSER_TRANSACTION, []byte("rwset"))
		expectedRWS := &fakeRWSet{namespaces: []cdriver.Namespace{"ns1"}}
		expectedTx := &fakeProcessTransaction{id: chdr.TxId, network: "network", channel: "channel"}
		handler := &fakeRWSetHandler{
			loadFn: func(payl *cb.Payload, header *cb.ChannelHeader) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
				require.Equal(t, chdr.TxId, header.TxId)
				require.NotNil(t, payl)
				return expectedRWS, expectedTx, nil
			},
		}

		loader := NewLoader(
			"network",
			"channel",
			&fakeEnvelopeService{
				existsFn: func(_ context.Context, _ string) bool { return true },
				loadFn:   func(_ context.Context, _ string) ([]byte, error) { return mustMarshalProto(t, env), nil },
			},
			nil,
			nil,
			nil,
		)
		err := loader.AddHandlerProvider(cb.HeaderType_ENDORSER_TRANSACTION, func(_, _ string, _ fdriver.RWSetInspector) fdriver.RWSetPayloadHandler {
			return handler
		})
		require.NoError(t, err)

		rws, tx, err := loader.GetRWSetFromEvn(t.Context(), cdriver.TxID(chdr.TxId))
		require.NoError(t, err)
		require.Same(t, expectedRWS, rws)
		require.Same(t, expectedTx, tx)
	})
}

func TestLoaderGetRWSetFromETx(t *testing.T) {
	t.Parallel()

	t.Run("transaction missing", func(t *testing.T) {
		t.Parallel()
		loader := NewLoader(
			"network",
			"channel",
			nil,
			&fakeTransactionService{existsFn: func(_ context.Context, _ string) bool { return false }},
			nil,
			nil,
		)
		_, _, err := loader.GetRWSetFromETx(t.Context(), "tx1")
		require.Error(t, err)
		require.ErrorContains(t, err, "transaction does not exist")
	})

	t.Run("load transaction error", func(t *testing.T) {
		t.Parallel()
		loader := NewLoader(
			"network",
			"channel",
			nil,
			&fakeTransactionService{
				existsFn: func(_ context.Context, _ string) bool { return true },
				loadFn:   func(_ context.Context, _ string) ([]byte, error) { return nil, stderrors.New("load-etx-failed") },
			},
			nil,
			nil,
		)
		_, _, err := loader.GetRWSetFromETx(t.Context(), "tx1")
		require.Error(t, err)
		require.ErrorContains(t, err, "cannot load etx")
	})

	t.Run("new transaction error", func(t *testing.T) {
		t.Parallel()
		loader := NewLoader(
			"network",
			"channel",
			nil,
			&fakeTransactionService{
				existsFn: func(_ context.Context, _ string) bool { return true },
				loadFn:   func(_ context.Context, _ string) ([]byte, error) { return []byte("tx-bytes"), nil },
			},
			&fakeTransactionManager{
				newFromBytesFn: func(_ context.Context, _ string, _ []byte) (fdriver.Transaction, error) {
					return nil, stderrors.New("new-tx-failed")
				},
			},
			nil,
		)
		_, _, err := loader.GetRWSetFromETx(t.Context(), "tx1")
		require.ErrorContains(t, err, "new-tx-failed")
	})

	t.Run("get rwset error", func(t *testing.T) {
		t.Parallel()
		loader := NewLoader(
			"network",
			"channel",
			nil,
			&fakeTransactionService{
				existsFn: func(_ context.Context, _ string) bool { return true },
				loadFn:   func(_ context.Context, _ string) ([]byte, error) { return []byte("tx-bytes"), nil },
			},
			&fakeTransactionManager{
				newFromBytesFn: func(_ context.Context, _ string, _ []byte) (fdriver.Transaction, error) {
					return &fakeTransaction{
						getRWSetFn: func() (fdriver.RWSet, error) {
							return nil, stderrors.New("get-rwset-failed")
						},
					}, nil
				},
			},
			nil,
		)
		_, _, err := loader.GetRWSetFromETx(t.Context(), "tx1")
		require.ErrorContains(t, err, "get-rwset-failed")
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		expectedRWS := &fakeRWSet{namespaces: []cdriver.Namespace{"ns1"}}
		expectedTx := &fakeTransaction{
			getRWSetFn: func() (fdriver.RWSet, error) { return expectedRWS, nil },
		}
		loader := NewLoader(
			"network",
			"channel",
			nil,
			&fakeTransactionService{
				existsFn: func(_ context.Context, _ string) bool { return true },
				loadFn:   func(_ context.Context, _ string) ([]byte, error) { return []byte("tx-bytes"), nil },
			},
			&fakeTransactionManager{
				newFromBytesFn: func(_ context.Context, _ string, raw []byte) (fdriver.Transaction, error) {
					require.Equal(t, []byte("tx-bytes"), raw)
					return expectedTx, nil
				},
			},
			nil,
		)
		rws, tx, err := loader.GetRWSetFromETx(t.Context(), "tx1")
		require.NoError(t, err)
		require.Same(t, expectedRWS, rws)
		require.Same(t, expectedTx, tx)
	})
}

func TestLoaderGetInspectingRWSetFromEvn(t *testing.T) {
	t.Parallel()

	t.Run("invalid envelope", func(t *testing.T) {
		t.Parallel()
		loader := NewLoader("network", "channel", nil, nil, nil, &fakeInspector{})
		_, _, err := loader.GetInspectingRWSetFromEvn(t.Context(), "tx1", []byte("bad-envelope"))
		require.Error(t, err)
		require.ErrorContains(t, err, "failed unmarshalling envelope")
	})

	t.Run("unpack error", func(t *testing.T) {
		t.Parallel()
		env, _, chdr, _, _, _ := buildTestEnvelope(t, cb.HeaderType_CONFIG, []byte("rwset"))
		loader := NewLoader("network", "channel", nil, nil, nil, &fakeInspector{})
		_, _, err := loader.GetInspectingRWSetFromEvn(t.Context(), cdriver.TxID(chdr.TxId), mustMarshalProto(t, env))
		require.Error(t, err)
		require.ErrorContains(t, err, "failed unpacking envelope")
	})

	t.Run("inspect error", func(t *testing.T) {
		t.Parallel()
		env, _, chdr, _, _, _ := buildTestEnvelope(t, cb.HeaderType_ENDORSER_TRANSACTION, []byte("rwset"))
		loader := NewLoader(
			"network",
			"channel",
			nil,
			nil,
			nil,
			&fakeInspector{
				inspectFunc: func(_ context.Context, _ []byte, _ ...cdriver.Namespace) (fdriver.RWSet, error) {
					return nil, stderrors.New("inspect-failed")
				},
			},
		)
		_, _, err := loader.GetInspectingRWSetFromEvn(t.Context(), cdriver.TxID(chdr.TxId), mustMarshalProto(t, env))
		require.ErrorContains(t, err, "inspect-failed")
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		results := []byte("rwset")
		env, _, chdr, _, _, _ := buildTestEnvelope(t, cb.HeaderType_ENDORSER_TRANSACTION, results)
		expectedRWS := &fakeRWSet{namespaces: []cdriver.Namespace{"ns1"}}
		loader := NewLoader(
			"network",
			"channel",
			nil,
			nil,
			nil,
			&fakeInspector{
				inspectFunc: func(_ context.Context, rwsetBytes []byte, _ ...cdriver.Namespace) (fdriver.RWSet, error) {
					require.Equal(t, results, rwsetBytes)
					return expectedRWS, nil
				},
			},
		)
		rws, tx, err := loader.GetInspectingRWSetFromEvn(t.Context(), cdriver.TxID(chdr.TxId), mustMarshalProto(t, env))
		require.NoError(t, err)
		require.Same(t, expectedRWS, rws)
		require.Equal(t, chdr.TxId, tx.ID())
		require.Equal(t, "network", tx.Network())
		require.Equal(t, "mychannel", tx.Channel())
	})
}
