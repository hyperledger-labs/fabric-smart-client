/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/protos"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func TestManager_NewTransaction(t *testing.T) {
	t.Parallel()
	m := transaction.NewManager()

	ctx := t.Context()
	creator := []byte("creator")
	nonce := []byte("nonce")
	txid := "txid"
	channel := "testchannel"
	rawRequest := []byte("request")

	// Error when factory not found
	_, err := m.NewTransaction(ctx, driver.EndorserTransaction, creator, nonce, txid, channel, rawRequest)
	require.ErrorContains(t, err, "transaction type [3] not recognized")

	// Add factory and test success
	mockFactory := &mock.TransactionFactory{}
	mockTx := &mock.Transaction{}
	mockFactory.NewTransactionReturns(mockTx, nil)

	m.AddTransactionFactory(driver.EndorserTransaction, mockFactory)

	tx, err := m.NewTransaction(ctx, driver.EndorserTransaction, creator, nonce, txid, channel, rawRequest)
	require.NoError(t, err)
	require.NotNil(t, tx)
}

func TestManager_NewTransactionFromBytes(t *testing.T) {
	t.Parallel()
	m := transaction.NewManager()
	ctx := t.Context()

	// Error on invalid JSON
	_, err := m.NewTransactionFromBytes(ctx, "testchannel", []byte("invalid json"))
	require.Error(t, err)

	// Error when factory not found
	validJSON, err := json.Marshal(&transaction.SerializedTransaction{Type: driver.EndorserTransaction, Raw: []byte("raw tx")})
	require.NoError(t, err)

	_, err = m.NewTransactionFromBytes(ctx, "testchannel", validJSON)
	require.ErrorContains(t, err, "transaction type [3] not recognized")

	// Add factory and test success
	mockFactory := &mock.TransactionFactory{}
	mockTx := &mock.Transaction{}
	mockFactory.NewTransactionReturns(mockTx, nil)
	mockTx.SetFromBytesReturns(nil)

	m.AddTransactionFactory(driver.EndorserTransaction, mockFactory)

	tx, err := m.NewTransactionFromBytes(ctx, "testchannel", validJSON)
	require.NoError(t, err)
	require.NotNil(t, tx)
}

// endorsedFabricTx returns the JSON of an endorsed Fabric transaction, as WrappedTransaction.Bytes
// produces it, the same transaction as an envelope, and a manager that decodes both.
func endorsedFabricTx(t *testing.T) (jsonRaw, envRaw []byte, m *transaction.Manager) {
	t.Helper()
	f := newEndorsableTx(t)
	require.NoError(t, f.tx.SetRWSet())
	require.NoError(t, f.tx.EndorseWithIdentity(view.Identity("endorser")))
	jsonRaw, err := (&transaction.WrappedTransaction{Transaction: f.tx, TransactionType: driver.EndorserTransaction}).Bytes()
	require.NoError(t, err)
	envRaw, err = protos.EnvelopeFromJSON(jsonRaw)
	require.NoError(t, err)
	m = transaction.NewManager()
	m.AddTransactionFactory(driver.EndorserTransaction, transaction.NewEndorserTransactionFactory("network", f.provider, f.sigService))
	return jsonRaw, envRaw, m
}

func TestNewTransactionFromBytesReadsEnvelope(t *testing.T) {
	t.Parallel()
	jsonRaw, envRaw, m := endorsedFabricTx(t)

	fromJSON, err := m.NewTransactionFromBytes(t.Context(), "channel", jsonRaw)
	require.NoError(t, err)
	fromEnv, err := m.NewTransactionFromBytes(t.Context(), "channel", envRaw)
	require.NoError(t, err)

	require.Equal(t, "txid", fromJSON.ID())
	require.Equal(t, fromJSON.ID(), fromEnv.ID())
	plainResults, err := fromJSON.Results()
	require.NoError(t, err)
	envResults, err := fromEnv.Results()
	require.NoError(t, err)
	require.Equal(t, []byte("rwset-bytes"), plainResults)
	require.Equal(t, plainResults, envResults)
}

func TestNewTransactionFromBytesRejectsOtherPlatform(t *testing.T) {
	t.Parallel()
	_, envRaw, m := endorsedFabricTx(t)
	env := &protos.TransactionEnvelope{}
	require.NoError(t, proto.Unmarshal(envRaw, env))
	env.Platform = "other"
	other, err := proto.Marshal(env)
	require.NoError(t, err)

	_, err = m.NewTransactionFromBytes(t.Context(), "channel", other)
	require.ErrorContains(t, err, "transaction for platform [other] on a [generic] network")
}

func TestEndorserTransactionFactory(t *testing.T) {
	t.Parallel()
	mockChannelProvider := &mock.ChannelProvider{}
	mockSigService := &mock.SignerService{}
	mockChannel := &mock.Channel{}

	mockChannelProvider.ChannelReturns(mockChannel, nil)

	factory := transaction.NewEndorserTransactionFactory("testnetwork", mockChannelProvider, mockSigService)

	ctx := t.Context()
	creator := []byte("creator")
	nonce := []byte("nonce")
	txid := "txid"
	channelName := "testchannel"
	rawRequest := []byte("request")

	tx, err := factory.NewTransaction(ctx, channelName, nonce, creator, txid, rawRequest)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, txid, tx.ID())
	require.Equal(t, channelName, tx.Channel())
	require.Equal(t, view.Identity(creator), tx.Creator())
	require.Equal(t, nonce, tx.Nonce())
	require.Equal(t, "testnetwork", tx.Network())
}

// keyedTransaction is a driver.Transaction bound to a channel with key, or whose
// key cannot be read if err is set.
type keyedTransaction struct {
	*mock.Transaction
	key *driver.ChannelKey
	err error
}

func (k *keyedTransaction) ChannelKey() (*driver.ChannelKey, error) { return k.key, k.err }

// Both encodings carry the transaction type and the key of its channel.
func TestWrappedTransaction_Bytes(t *testing.T) {
	t.Parallel()
	key := &driver.ChannelKey{Platform: "generic", Channel: "ch1", GenesisHash: []byte("genesis")}
	mockTx := &mock.Transaction{}
	mockTx.BytesReturns([]byte("raw transaction"), nil)
	mockTx.BytesNoTransientReturns([]byte("raw no transient"), nil)
	wt := transaction.WrappedTransaction{Transaction: &keyedTransaction{Transaction: mockTx, key: key}, TransactionType: driver.EndorserTransaction}

	for _, c := range []struct {
		encode func() ([]byte, error)
		want   string
	}{
		{wt.Bytes, "raw transaction"},
		{wt.BytesNoTransient, "raw no transient"},
	} {
		b, err := c.encode()
		require.NoError(t, err)
		var st transaction.SerializedTransaction
		require.NoError(t, json.Unmarshal(b, &st))
		require.Equal(t, driver.EndorserTransaction, st.Type)
		require.Equal(t, []byte(c.want), st.Raw)
		require.Equal(t, key, st.Key)
	}
}

// A transaction whose key cannot be read yet serializes without one, so it can
// still be stored and hashed locally.
func TestWrappedTransaction_BytesWhileKeyUnreadable(t *testing.T) {
	t.Parallel()
	mockTx := &mock.Transaction{}
	mockTx.BytesReturns([]byte("raw transaction"), nil)
	wt := transaction.WrappedTransaction{Transaction: &keyedTransaction{Transaction: mockTx, err: errors.New("ledger unavailable")}, TransactionType: driver.EndorserTransaction}

	b, err := wt.Bytes()
	require.NoError(t, err)
	require.NotContains(t, string(b), `"Key"`)
}

// A transaction that exposes no key serializes without one.
func TestWrappedTransaction_BytesWithoutKey(t *testing.T) {
	t.Parallel()
	mockTx := &mock.Transaction{}
	mockTx.BytesReturns([]byte("raw transaction"), nil)
	wt := transaction.WrappedTransaction{Transaction: mockTx, TransactionType: driver.EndorserTransaction}

	b, err := wt.Bytes()
	require.NoError(t, err)
	require.NotContains(t, string(b), `"Key"`)
}

func TestManager_ComputeTxID(t *testing.T) {
	t.Parallel()
	m := transaction.NewManager()
	id := m.ComputeTxID(&driver.TxIDComponents{Nonce: []byte("nonce"), Creator: []byte("creator")})
	require.NotEmpty(t, id)
}

func TestManager_NewEnvelope(t *testing.T) {
	t.Parallel()
	m := transaction.NewManager()
	env := m.NewEnvelope()
	require.NotNil(t, env)
}

func TestManager_NewProposalResponseFromBytes(t *testing.T) {
	t.Parallel()
	m := transaction.NewManager()
	_, err := m.NewProposalResponseFromBytes([]byte("invalid"))
	require.Error(t, err)
}

func TestManager_NewTransactionFromEnvelopeBytes(t *testing.T) {
	t.Parallel()
	m := transaction.NewManager()
	ctx := t.Context()
	_, err := m.NewTransactionFromEnvelopeBytes(ctx, "testchannel", []byte("invalid"))
	require.Error(t, err)
}

func TestManager_NewProcessedTransactionFromEnvelopePayload(t *testing.T) {
	t.Parallel()
	m := transaction.NewManager()
	_, _, err := m.NewProcessedTransactionFromEnvelopePayload([]byte("invalid"))
	require.Error(t, err)
}

func TestManager_NewProcessedTransactionFromEnvelopeRaw(t *testing.T) {
	t.Parallel()
	m := transaction.NewManager()
	_, err := m.NewProcessedTransactionFromEnvelopeRaw([]byte("invalid"))
	require.Error(t, err)
}

func TestManager_NewProcessedTransaction(t *testing.T) {
	t.Parallel()
	m := transaction.NewManager()
	_, err := m.NewProcessedTransaction([]byte("invalid"))
	require.Error(t, err)
}

func TestManager_NewTransactionFromEnvelopeBytesDispatch(t *testing.T) {
	t.Parallel()
	raw := mustMarshal(t, createValidEnvelope(t))
	factoryErr := errors.New("factory down")
	setErr := errors.New("bad envelope")
	tests := []struct {
		name    string
		setup   func(m *transaction.Manager, factory *mock.TransactionFactory, tx *mock.Transaction)
		wantErr error
	}{
		{
			name: "success",
			setup: func(m *transaction.Manager, factory *mock.TransactionFactory, _ *mock.Transaction) {
				m.AddTransactionFactory(driver.EndorserTransaction, factory)
			},
		},
		{
			name: "factory fails",
			setup: func(m *transaction.Manager, factory *mock.TransactionFactory, _ *mock.Transaction) {
				m.AddTransactionFactory(driver.EndorserTransaction, factory)
				factory.NewTransactionReturns(nil, factoryErr)
			},
			wantErr: factoryErr,
		},
		{
			name: "envelope is rejected",
			setup: func(m *transaction.Manager, factory *mock.TransactionFactory, tx *mock.Transaction) {
				m.AddTransactionFactory(driver.EndorserTransaction, factory)
				tx.SetFromEnvelopeBytesReturns(setErr)
			},
			wantErr: setErr,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := transaction.NewManager()
			factory := &mock.TransactionFactory{}
			tx := &mock.Transaction{}
			factory.NewTransactionReturns(tx, nil)
			tc.setup(m, factory, tx)

			got, err := m.NewTransactionFromEnvelopeBytes(t.Context(), "testchannel", raw)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.Nil(t, got)
				return
			}
			require.NoError(t, err)
			wrapped, ok := got.(*transaction.WrappedTransaction)
			require.True(t, ok)
			require.Same(t, tx, wrapped.Transaction)
			require.Equal(t, driver.EndorserTransaction, wrapped.TransactionType)
			_, channel, _, _, _, _ := factory.NewTransactionArgsForCall(0)
			require.Equal(t, "testchannel", channel)
			require.Equal(t, raw, tx.SetFromEnvelopeBytesArgsForCall(0))
		})
	}

	t.Run("unregistered type", func(t *testing.T) {
		t.Parallel()
		_, err := transaction.NewManager().NewTransactionFromEnvelopeBytes(t.Context(), "testchannel", raw)
		require.EqualError(t, err, "transaction type [3] not recognized")
	})
}

func TestManager_FactoryErrors(t *testing.T) {
	t.Parallel()
	factoryErr := errors.New("factory down")
	newManager := func(tx driver.Transaction, err error) *transaction.Manager {
		factory := &mock.TransactionFactory{}
		factory.NewTransactionReturns(tx, err)
		m := transaction.NewManager()
		m.AddTransactionFactory(driver.EndorserTransaction, factory)
		return m
	}
	serialized, err := json.Marshal(&transaction.SerializedTransaction{Type: driver.EndorserTransaction, Raw: []byte("raw tx")})
	require.NoError(t, err)

	t.Run("NewTransaction", func(t *testing.T) {
		t.Parallel()
		tx, err := newManager(nil, factoryErr).NewTransaction(t.Context(), driver.EndorserTransaction, nil, nil, "", "ch", nil)
		require.ErrorIs(t, err, factoryErr)
		require.Nil(t, tx)
	})

	t.Run("NewTransactionFromBytes factory fails", func(t *testing.T) {
		t.Parallel()
		tx, err := newManager(nil, factoryErr).NewTransactionFromBytes(t.Context(), "ch", serialized)
		require.ErrorIs(t, err, factoryErr)
		require.Nil(t, tx)
	})

	t.Run("NewTransactionFromBytes payload is rejected", func(t *testing.T) {
		t.Parallel()
		inner := &mock.Transaction{}
		setErr := errors.New("bad payload")
		inner.SetFromBytesReturns(setErr)
		tx, err := newManager(inner, nil).NewTransactionFromBytes(t.Context(), "ch", serialized)
		require.ErrorIs(t, err, setErr)
		require.Nil(t, tx)
		require.Equal(t, []byte("raw tx"), inner.SetFromBytesArgsForCall(0))
	})

	t.Run("EndorserTransactionFactory without a channel", func(t *testing.T) {
		t.Parallel()
		provider := &mock.ChannelProvider{}
		channelErr := errors.New("unknown channel")
		provider.ChannelReturns(nil, channelErr)
		factory := transaction.NewEndorserTransactionFactory("network", provider, &mock.SignerService{})
		tx, err := factory.NewTransaction(t.Context(), "ch", nil, nil, "", nil)
		require.ErrorIs(t, err, channelErr)
		require.Nil(t, tx)
	})

	t.Run("WrappedTransaction.Bytes", func(t *testing.T) {
		t.Parallel()
		inner := &mock.Transaction{}
		bytesErr := errors.New("marshal failed")
		inner.BytesReturns(nil, bytesErr)
		wt := transaction.WrappedTransaction{Transaction: inner, TransactionType: driver.EndorserTransaction}
		_, err := wt.Bytes()
		require.ErrorIs(t, err, bytesErr)
	})
}
