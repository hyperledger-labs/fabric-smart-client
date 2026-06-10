/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rwset

import (
	stderrors "errors"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/require"

	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	rwsetfake "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/rwset/fake"
	rwsetmock "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/rwset/mock"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

func TestEndorserTransactionHandlerLoad(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		results := []byte("rwset-bytes")
		_, payl, chdr, _, _, _ := buildTestEnvelope(t, cb.HeaderType_ENDORSER_TRANSACTION, results)
		expectedRWS := &rwsetfake.RWSet{NamespacesList: []cdriver.Namespace{"ns1"}}
		inspector := &rwsetmock.RWSetInspector{}
		inspector.NewRWSetFromBytesReturns(expectedRWS, nil)

		h := NewEndorserTransactionHandler("test-network", "mychannel", inspector)
		rws, tx, err := h.Load(payl, chdr)
		require.NoError(t, err)
		require.Same(t, expectedRWS, rws)
		_, txID, rwset := inspector.NewRWSetFromBytesArgsForCall(0)
		require.Equal(t, cdriver.TxID(chdr.TxId), txID)
		require.Equal(t, results, rwset)
		require.Equal(t, chdr.TxId, tx.ID())
		require.Equal(t, "test-network", tx.Network())
		require.Equal(t, "mychannel", tx.Channel())
	})

	t.Run("unpack error", func(t *testing.T) {
		t.Parallel()
		_, payl, chdr, _, _, _ := buildTestEnvelope(t, cb.HeaderType_CONFIG, []byte("rwset"))
		h := NewEndorserTransactionHandler("test-network", "mychannel", &rwsetmock.RWSetInspector{})
		_, _, err := h.Load(payl, chdr)
		require.Error(t, err)
		require.ErrorContains(t, err, "failed unpacking envelope")
	})

	t.Run("rwset inspector error", func(t *testing.T) {
		t.Parallel()
		_, payl, chdr, _, _, _ := buildTestEnvelope(t, cb.HeaderType_ENDORSER_TRANSACTION, []byte("rwset"))
		inspector := &rwsetmock.RWSetInspector{}
		inspector.NewRWSetFromBytesReturns(nil, stderrors.New("new-rwset-failed"))
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
		return &rwsetmock.RWSetPayloadHandler{}
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
		envelopeService := &rwsetfake.EnvelopeService{}
		loader := NewLoader(
			"network",
			"channel",
			envelopeService,
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
		envelopeService := &rwsetfake.EnvelopeService{ExistsValue: true, LoadErr: stderrors.New("load-failed")}
		loader := NewLoader(
			"network",
			"channel",
			envelopeService,
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
		envelopeService := &rwsetfake.EnvelopeService{ExistsValue: true, Envelope: []byte("bad-envelope")}
		loader := NewLoader(
			"network",
			"channel",
			envelopeService,
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
		envelopeService := &rwsetfake.EnvelopeService{ExistsValue: true, Envelope: mustMarshalProto(t, env)}
		loader := NewLoader(
			"network",
			"mychannel",
			envelopeService,
			nil,
			nil,
			nil,
		)
		_, _, err := loader.GetRWSetFromEvn(t.Context(), cdriver.TxID(chdr.TxId))
		require.Error(t, err)
		require.ErrorContains(t, err, "header type not supported")
	})

	t.Run("channel mismatch", func(t *testing.T) {
		t.Parallel()
		env, _, chdr, _, _, _ := buildTestEnvelope(t, cb.HeaderType_ENDORSER_TRANSACTION, []byte("rwset"))
		envelopeService := &rwsetfake.EnvelopeService{ExistsValue: true, Envelope: mustMarshalProto(t, env)}
		loader := NewLoader(
			"network",
			"channel",
			envelopeService,
			nil,
			nil,
			nil,
		)
		_, _, err := loader.GetRWSetFromEvn(t.Context(), cdriver.TxID(chdr.TxId))
		require.ErrorContains(t, err, "channel mismatch, expected [channel], got [mychannel]")
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		env, _, chdr, _, _, _ := buildTestEnvelope(t, cb.HeaderType_ENDORSER_TRANSACTION, []byte("rwset"))
		expectedRWS := &rwsetfake.RWSet{NamespacesList: []cdriver.Namespace{"ns1"}}
		expectedTx := &rwsetfake.ProcessTransaction{IDValue: chdr.TxId, NetworkValue: "network", ChannelValue: "mychannel"}
		handler := &rwsetmock.RWSetPayloadHandler{}
		handler.LoadReturns(expectedRWS, expectedTx, nil)
		envelopeService := &rwsetfake.EnvelopeService{ExistsValue: true, Envelope: mustMarshalProto(t, env)}

		loader := NewLoader(
			"network",
			"mychannel",
			envelopeService,
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
		payl, header := handler.LoadArgsForCall(0)
		require.Equal(t, chdr.TxId, header.TxId)
		require.NotNil(t, payl)
	})
}

func TestLoaderGetRWSetFromETx(t *testing.T) {
	t.Parallel()

	t.Run("transaction missing", func(t *testing.T) {
		t.Parallel()
		transactionService := &rwsetfake.EndorserTransactionService{}
		loader := NewLoader(
			"network",
			"channel",
			nil,
			transactionService,
			nil,
			nil,
		)
		_, _, err := loader.GetRWSetFromETx(t.Context(), "tx1")
		require.Error(t, err)
		require.ErrorContains(t, err, "transaction does not exist")
	})

	t.Run("load transaction error", func(t *testing.T) {
		t.Parallel()
		transactionService := &rwsetfake.EndorserTransactionService{ExistsValue: true, LoadErr: stderrors.New("load-etx-failed")}
		loader := NewLoader(
			"network",
			"channel",
			nil,
			transactionService,
			nil,
			nil,
		)
		_, _, err := loader.GetRWSetFromETx(t.Context(), "tx1")
		require.Error(t, err)
		require.ErrorContains(t, err, "cannot load etx")
	})

	t.Run("new transaction error", func(t *testing.T) {
		t.Parallel()
		transactionService := &rwsetfake.EndorserTransactionService{ExistsValue: true, Transaction: []byte("tx-bytes")}
		transactionManager := &rwsetmock.TransactionManager{}
		transactionManager.NewTransactionFromBytesReturns(nil, stderrors.New("new-tx-failed"))
		loader := NewLoader(
			"network",
			"channel",
			nil,
			transactionService,
			transactionManager,
			nil,
		)
		_, _, err := loader.GetRWSetFromETx(t.Context(), "tx1")
		require.ErrorContains(t, err, "new-tx-failed")
	})

	t.Run("get rwset error", func(t *testing.T) {
		t.Parallel()
		transactionService := &rwsetfake.EndorserTransactionService{ExistsValue: true, Transaction: []byte("tx-bytes")}
		transactionManager := &rwsetmock.TransactionManager{}
		transactionManager.NewTransactionFromBytesReturns(&rwsetfake.Transaction{RWSetErr: stderrors.New("get-rwset-failed")}, nil)
		loader := NewLoader(
			"network",
			"channel",
			nil,
			transactionService,
			transactionManager,
			nil,
		)
		_, _, err := loader.GetRWSetFromETx(t.Context(), "tx1")
		require.ErrorContains(t, err, "get-rwset-failed")
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		expectedRWS := &rwsetfake.RWSet{NamespacesList: []cdriver.Namespace{"ns1"}}
		expectedTx := &rwsetfake.Transaction{RWSetValue: expectedRWS}
		transactionService := &rwsetfake.EndorserTransactionService{ExistsValue: true, Transaction: []byte("tx-bytes")}
		transactionManager := &rwsetmock.TransactionManager{}
		transactionManager.NewTransactionFromBytesReturns(expectedTx, nil)
		loader := NewLoader(
			"network",
			"channel",
			nil,
			transactionService,
			transactionManager,
			nil,
		)
		rws, tx, err := loader.GetRWSetFromETx(t.Context(), "tx1")
		require.NoError(t, err)
		require.Same(t, expectedRWS, rws)
		require.Same(t, expectedTx, tx)
		_, _, raw := transactionManager.NewTransactionFromBytesArgsForCall(0)
		require.Equal(t, []byte("tx-bytes"), raw)
	})
}

func TestLoaderGetInspectingRWSetFromEvn(t *testing.T) {
	t.Parallel()

	t.Run("invalid envelope", func(t *testing.T) {
		t.Parallel()
		loader := NewLoader("network", "channel", nil, nil, nil, &rwsetmock.RWSetInspector{})
		_, _, err := loader.GetInspectingRWSetFromEvn(t.Context(), "tx1", []byte("bad-envelope"))
		require.Error(t, err)
		require.ErrorContains(t, err, "failed unmarshalling envelope")
	})

	t.Run("unpack error", func(t *testing.T) {
		t.Parallel()
		env, _, chdr, _, _, _ := buildTestEnvelope(t, cb.HeaderType_CONFIG, []byte("rwset"))
		loader := NewLoader("network", "channel", nil, nil, nil, &rwsetmock.RWSetInspector{})
		_, _, err := loader.GetInspectingRWSetFromEvn(t.Context(), cdriver.TxID(chdr.TxId), mustMarshalProto(t, env))
		require.Error(t, err)
		require.ErrorContains(t, err, "failed unpacking envelope")
	})

	t.Run("inspect error", func(t *testing.T) {
		t.Parallel()
		env, _, chdr, _, _, _ := buildTestEnvelope(t, cb.HeaderType_ENDORSER_TRANSACTION, []byte("rwset"))
		inspector := &rwsetmock.RWSetInspector{}
		inspector.InspectRWSetReturns(nil, stderrors.New("inspect-failed"))
		loader := NewLoader(
			"network",
			"mychannel",
			nil,
			nil,
			nil,
			inspector,
		)
		_, _, err := loader.GetInspectingRWSetFromEvn(t.Context(), cdriver.TxID(chdr.TxId), mustMarshalProto(t, env))
		require.ErrorContains(t, err, "inspect-failed")
	})

	t.Run("channel mismatch", func(t *testing.T) {
		t.Parallel()
		env, _, chdr, _, _, _ := buildTestEnvelope(t, cb.HeaderType_ENDORSER_TRANSACTION, []byte("rwset"))
		loader := NewLoader("network", "channel", nil, nil, nil, &rwsetmock.RWSetInspector{})
		_, _, err := loader.GetInspectingRWSetFromEvn(t.Context(), cdriver.TxID(chdr.TxId), mustMarshalProto(t, env))
		require.ErrorContains(t, err, "channel mismatch, expected [channel], got [mychannel]")
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		results := []byte("rwset")
		env, _, chdr, _, _, _ := buildTestEnvelope(t, cb.HeaderType_ENDORSER_TRANSACTION, results)
		expectedRWS := &rwsetfake.RWSet{NamespacesList: []cdriver.Namespace{"ns1"}}
		inspector := &rwsetmock.RWSetInspector{}
		inspector.InspectRWSetReturns(expectedRWS, nil)
		loader := NewLoader(
			"network",
			"mychannel",
			nil,
			nil,
			nil,
			inspector,
		)
		rws, tx, err := loader.GetInspectingRWSetFromEvn(t.Context(), cdriver.TxID(chdr.TxId), mustMarshalProto(t, env))
		require.NoError(t, err)
		require.Same(t, expectedRWS, rws)
		_, rwsetBytes, _ := inspector.InspectRWSetArgsForCall(0)
		require.Equal(t, results, rwsetBytes)
		require.Equal(t, chdr.TxId, tx.ID())
		require.Equal(t, "network", tx.Network())
		require.Equal(t, "mychannel", tx.Channel())
	})
}
