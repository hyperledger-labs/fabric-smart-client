/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
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
	validJson, err := json.Marshal(&transaction.SerializedTransaction{Type: driver.EndorserTransaction, Raw: []byte("raw tx")})
	require.NoError(t, err)

	_, err = m.NewTransactionFromBytes(ctx, "testchannel", validJson)
	require.ErrorContains(t, err, "transaction type [3] not recognized")

	// Add factory and test success
	mockFactory := &mock.TransactionFactory{}
	mockTx := &mock.Transaction{}
	mockFactory.NewTransactionReturns(mockTx, nil)
	mockTx.SetFromBytesReturns(nil)

	m.AddTransactionFactory(driver.EndorserTransaction, mockFactory)

	tx, err := m.NewTransactionFromBytes(ctx, "testchannel", validJson)
	require.NoError(t, err)
	require.NotNil(t, tx)
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

func TestWrappedTransaction_Bytes(t *testing.T) {
	t.Parallel()
	mockTx := &mock.Transaction{}
	mockTx.BytesReturns([]byte("raw transaction"), nil)

	wt := transaction.WrappedTransaction{Transaction: mockTx, TransactionType: driver.EndorserTransaction}
	b, err := wt.Bytes()
	require.NoError(t, err)

	var st transaction.SerializedTransaction
	err = json.Unmarshal(b, &st)
	require.NoError(t, err)
	require.Equal(t, driver.EndorserTransaction, st.Type)
	require.Equal(t, []byte("raw transaction"), st.Raw)
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
