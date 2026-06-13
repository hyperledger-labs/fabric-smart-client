/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction_test

import (
	"context"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

func TestMetadataService(t *testing.T) { //nolint:paralleltest
	mockStore := &mock.MetadataStore{}
	mds := transaction.NewMetadataService(mockStore, "network", "channel")

	ctx := context.Background()

	// Exists
	mockStore.ExistMetadataReturns(true, nil)
	require.True(t, mds.Exists(ctx, "txid"))

	// StoreTransient
	tm := map[string][]byte{"key": []byte("value")}
	mockStore.PutMetadataReturns(nil)
	err := mds.StoreTransient(ctx, "txid", tm)
	require.NoError(t, err)

	// LoadTransient
	mockStore.GetMetadataReturns(tm, nil)
	loaded, err := mds.LoadTransient(ctx, "txid")
	require.NoError(t, err)
	require.Equal(t, driver.TransientMap(tm), loaded)
}

func TestEnvelopeService(t *testing.T) { //nolint:paralleltest
	mockStore := &mock.EnvelopeStore{}
	envs := transaction.NewEnvelopeService(mockStore, "network", "channel")

	ctx := context.Background()

	// Exists
	mockStore.ExistsEnvelopeReturns(true, nil)
	require.True(t, envs.Exists(ctx, "txid"))

	// StoreEnvelope with byte slice
	mockStore.PutEnvelopeReturns(nil)
	err := envs.StoreEnvelope(ctx, "txid", []byte("envelope"))
	require.NoError(t, err)

	// StoreEnvelope with common.Envelope
	env := &common.Envelope{Payload: []byte("payload")}
	err = envs.StoreEnvelope(ctx, "txid", env)
	require.NoError(t, err)

	// StoreEnvelope invalid
	err = envs.StoreEnvelope(ctx, "txid", "invalid string")
	require.ErrorContains(t, err, "invalid env")

	// LoadEnvelope
	mockStore.GetEnvelopeReturns([]byte("envelope"), nil)
	loaded, err := envs.LoadEnvelope(ctx, "txid")
	require.NoError(t, err)
	require.Equal(t, []byte("envelope"), loaded)
}

func TestEndorseTransactionService(t *testing.T) { //nolint:paralleltest
	mockStore := &mock.EndorseTxStore{}
	ets := transaction.NewEndorseTransactionService(mockStore, "network", "channel")

	ctx := context.Background()

	// Exists
	mockStore.ExistsEndorseTxReturns(true, nil)
	require.True(t, ets.Exists(ctx, "txid"))

	// StoreTransaction
	mockStore.PutEndorseTxReturns(nil)
	err := ets.StoreTransaction(ctx, "txid", []byte("env"))
	require.NoError(t, err)

	// LoadTransaction
	mockStore.GetEndorseTxReturns([]byte("env"), nil)
	loaded, err := ets.LoadTransaction(ctx, "txid")
	require.NoError(t, err)
	require.Equal(t, []byte("env"), loaded)
}
