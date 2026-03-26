/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"context"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/transaction/mocks"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/hyperledger/fabric-x-common/api/committerpb"
	"github.com/stretchr/testify/require"
)

type stubTransactionFactory struct {
	newTransaction func(ctx context.Context, channel string, nonce, creator []byte, txID driver2.TxID, rawRequest []byte) (driver.Transaction, error)
}

func (s *stubTransactionFactory) NewTransaction(ctx context.Context, channel string, nonce, creator []byte, txID driver2.TxID, rawRequest []byte) (driver.Transaction, error) {
	return s.newTransaction(ctx, channel, nonce, creator, txID, rawRequest)
}

func TestManagerBasics(t *testing.T) {
	m := NewManager()
	require.NotNil(t, m)

	id := &driver.TxIDComponents{Nonce: []byte("nonce"), Creator: []byte("creator")}
	require.Equal(t, transaction.ComputeTxID(id), m.ComputeTxID(id))

	env := m.NewEnvelope()
	require.NotNil(t, env)
	require.Equal(t, "", env.TxID())
	require.Equal(t, []byte(nil), env.Nonce())
	require.Equal(t, []byte(nil), env.Creator())
	require.Equal(t, []byte(nil), env.Results())

	rawPR, err := proto.Marshal(&peer.ProposalResponse{Payload: []byte("payload")})
	require.NoError(t, err)

	pr, err := m.NewProposalResponseFromBytes(rawPR)
	require.NoError(t, err)
	require.Equal(t, []byte("payload"), pr.Payload())
}

func TestManagerTransactionFactory(t *testing.T) {
	t.Run("missing factory", func(t *testing.T) {
		m := NewManager()

		_, err := m.transactionFactory(driver.EndorserTransaction)
		require.Error(t, err)
		require.Contains(t, err.Error(), "no transaction factory found")
	})

	t.Run("wrong stored type panics", func(t *testing.T) {
		m := NewManager()
		m.transactionFactories.Store(driver.EndorserTransaction, "not-a-factory")

		require.PanicsWithValue(t, "retrieved factory is not driver.TransactionFactory", func() {
			_, _ = m.transactionFactory(driver.EndorserTransaction)
		})
	})

	t.Run("returns stored factory", func(t *testing.T) {
		m := NewManager()
		expected := &stubTransactionFactory{}
		m.AddTransactionFactory(driver.EndorserTransaction, expected)

		factory, err := m.transactionFactory(driver.EndorserTransaction)
		require.NoError(t, err)
		require.Same(t, expected, factory)
	})
}

func TestManagerNewTransaction(t *testing.T) {
	ctx := context.Background()
	creator := view.Identity([]byte("creator"))
	nonce := []byte("nonce")
	rawRequest := []byte("request")

	t.Run("factory lookup fails", func(t *testing.T) {
		m := NewManager()

		tx, err := m.NewTransaction(ctx, driver.EndorserTransaction, creator, nonce, "tx1", "ch1", rawRequest)
		require.Nil(t, tx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "no transaction factory found")
	})

	t.Run("delegates to factory", func(t *testing.T) {
		m := NewManager()
		expectedTx := &Transaction{TTxID: "tx1", TChannel: "ch1"}

		factory := &stubTransactionFactory{
			newTransaction: func(gotCtx context.Context, channel string, gotNonce, gotCreator []byte, txID driver2.TxID, gotRawRequest []byte) (driver.Transaction, error) {
				require.Equal(t, ctx, gotCtx)
				require.Equal(t, "ch1", channel)
				require.Equal(t, nonce, gotNonce)
				require.Equal(t, []byte(creator), gotCreator)
				require.Equal(t, driver2.TxID("tx1"), txID)
				require.Equal(t, rawRequest, gotRawRequest)
				return expectedTx, nil
			},
		}
		m.AddTransactionFactory(driver.EndorserTransaction, factory)

		tx, err := m.NewTransaction(ctx, driver.EndorserTransaction, creator, nonce, "tx1", "ch1", rawRequest)
		require.NoError(t, err)
		require.Same(t, expectedTx, tx)
	})
}

func TestManagerNewTransactionFromBytes(t *testing.T) {
	ctx := context.Background()

	t.Run("missing default factory", func(t *testing.T) {
		m := NewManager()

		tx, err := m.NewTransactionFromBytes(ctx, "ch1", []byte(`{"TTxID":"tx1"}`))
		require.Nil(t, tx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "no transaction factory found")
	})

	t.Run("factory creation fails", func(t *testing.T) {
		m := NewManager()
		m.AddTransactionFactory(driver.EndorserTransaction, &stubTransactionFactory{
			newTransaction: func(context.Context, string, []byte, []byte, driver2.TxID, []byte) (driver.Transaction, error) {
				return nil, assertErr("boom")
			},
		})

		tx, err := m.NewTransactionFromBytes(ctx, "ch1", []byte(`{"TTxID":"tx1"}`))
		require.Nil(t, tx)
		require.EqualError(t, err, "boom")
	})

	t.Run("SetFromBytes fails", func(t *testing.T) {
		m := NewManager()
		m.AddTransactionFactory(driver.EndorserTransaction, &stubTransactionFactory{
			newTransaction: func(context.Context, string, []byte, []byte, driver2.TxID, []byte) (driver.Transaction, error) {
				return &Transaction{fns: &mocks.FakeFabricNetworkService{ChannelStub: func(string) (driver.Channel, error) { return nil, nil }}}, nil
			},
		})

		tx, err := m.NewTransactionFromBytes(ctx, "ch1", []byte("not-json"))
		require.Nil(t, tx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "json unmarshal from bytes")
	})

	t.Run("success", func(t *testing.T) {
		m := NewManager()
		m.AddTransactionFactory(driver.EndorserTransaction, &stubTransactionFactory{
			newTransaction: func(context.Context, string, []byte, []byte, driver2.TxID, []byte) (driver.Transaction, error) {
				return &Transaction{fns: &mocks.FakeFabricNetworkService{ChannelStub: func(string) (driver.Channel, error) { return nil, nil }}}, nil
			},
		})

		raw := []byte(`{"TTxID":"tx1","TChannel":"ch1","TNetwork":"net1","TTransient":{"k":"dg=="}}`)
		tx, err := m.NewTransactionFromBytes(ctx, "ignored-channel", raw)
		require.NoError(t, err)

		concrete, ok := tx.(*Transaction)
		require.True(t, ok)
		require.Equal(t, "tx1", concrete.ID())
		require.Equal(t, "ch1", concrete.Channel())
		require.Equal(t, "net1", concrete.Network())
		require.Equal(t, driver.TransientMap{"k": []byte("v")}, concrete.Transient())
	})
}

func TestManagerNewTransactionFromEnvelopeBytesPanics(t *testing.T) {
	m := NewManager()

	require.PanicsWithValue(t, "NewTransactionFromEnvelopeBytes >> implement me", func() {
		_, _ = m.NewTransactionFromEnvelopeBytes(context.Background(), "ch1", []byte("env"))
	})
}

func TestManagerProcessedTransactionDelegation(t *testing.T) {
	m := NewManager()
	payloadRaw := mustEnvelopePayloadBytes(t, "tx1", []byte("results"))
	envRaw := mustEnvelopeBytes(t, payloadRaw)
	ptRaw := mustProcessedTransactionBytes(t, int32(committerpb.Status_COMMITTED), payloadRaw)

	ptFromPayload, headerType, err := m.NewProcessedTransactionFromEnvelopePayload(payloadRaw)
	require.NoError(t, err)
	require.Equal(t, int32(common.HeaderType_MESSAGE), headerType)
	require.Equal(t, "tx1", ptFromPayload.TxID())
	require.Equal(t, []byte("results"), ptFromPayload.Results())

	ptFromEnv, err := m.NewProcessedTransactionFromEnvelopeRaw(envRaw)
	require.NoError(t, err)
	require.Equal(t, "tx1", ptFromEnv.TxID())
	require.Equal(t, []byte("results"), ptFromEnv.Results())
	require.Equal(t, envRaw, ptFromEnv.Envelope())

	pt, err := m.NewProcessedTransaction(ptRaw)
	require.NoError(t, err)
	require.Equal(t, "tx1", pt.TxID())
	require.Equal(t, []byte("results"), pt.Results())
	require.Equal(t, int32(committerpb.Status_COMMITTED), pt.ValidationCode())
	require.True(t, pt.IsValid())
	require.NotEmpty(t, pt.Envelope())
}

func TestNewProcessedTransactionConstructorsAndAccessors(t *testing.T) {
	t.Run("from envelope payload invalid payload", func(t *testing.T) {
		pt, headerType, err := NewProcessedTransactionFromEnvelopePayload([]byte("not-a-payload"))
		require.Nil(t, pt)
		require.Equal(t, int32(-1), headerType)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to unmarshal payload")
	})

	t.Run("from envelope raw invalid envelope", func(t *testing.T) {
		pt, err := NewProcessedTransactionFromEnvelopeRaw([]byte("not-an-envelope"))
		require.Nil(t, pt)
		require.Error(t, err)
	})

	t.Run("from processed transaction invalid protobuf", func(t *testing.T) {
		pt, err := NewProcessedTransaction([]byte("not-a-processed-tx"))
		require.Nil(t, pt)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unmarshal failed")
	})

	t.Run("from processed transaction invalid embedded envelope", func(t *testing.T) {
		raw, err := proto.Marshal(&peer.ProcessedTransaction{
			ValidationCode:      int32(committerpb.Status_COMMITTED),
			TransactionEnvelope: &common.Envelope{Payload: []byte("not-a-payload")},
		})
		require.NoError(t, err)

		pt, err := NewProcessedTransaction(raw)
		require.Nil(t, pt)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to unmarshal payload")
	})

	t.Run("is valid false when validation code differs", func(t *testing.T) {
		payloadRaw := mustEnvelopePayloadBytes(t, "tx2", []byte("results-2"))
		raw := mustProcessedTransactionBytes(t, int32(committerpb.Status_ABORTED_SIGNATURE_INVALID), payloadRaw)

		pt, err := NewProcessedTransaction(raw)
		require.NoError(t, err)
		require.Equal(t, "tx2", pt.TxID())
		require.Equal(t, []byte("results-2"), pt.Results())
		require.Equal(t, int32(committerpb.Status_ABORTED_SIGNATURE_INVALID), pt.ValidationCode())
		require.False(t, pt.IsValid())
		require.NotEmpty(t, pt.Envelope())
	})
}

func mustEnvelopePayloadBytes(t *testing.T, txID string, results []byte) []byte {
	t.Helper()

	chdr, err := proto.Marshal(&common.ChannelHeader{
		Type: int32(common.HeaderType_MESSAGE),
		TxId: txID,
	})
	require.NoError(t, err)

	payload, err := proto.Marshal(&common.Payload{
		Header: &common.Header{ChannelHeader: chdr},
		Data:   results,
	})
	require.NoError(t, err)

	return payload
}

func mustEnvelopeBytes(t *testing.T, payloadRaw []byte) []byte {
	t.Helper()

	raw, err := proto.Marshal(&common.Envelope{Payload: payloadRaw})
	require.NoError(t, err)
	return raw
}

func mustProcessedTransactionBytes(t *testing.T, validationCode int32, payloadRaw []byte) []byte {
	t.Helper()

	raw, err := proto.Marshal(&peer.ProcessedTransaction{
		ValidationCode:      validationCode,
		TransactionEnvelope: &common.Envelope{Payload: payloadRaw},
	})
	require.NoError(t, err)
	return raw
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
