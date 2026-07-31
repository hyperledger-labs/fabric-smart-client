/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction_test

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

func TestMetadataService(t *testing.T) {
	t.Parallel()
	mockStore := &mock.MetadataStore{}
	mds := transaction.NewMetadataService(mockStore, "network", "channel")

	ctx := t.Context()

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

func TestFieldMappingRoundTrip(t *testing.T) {
	t.Parallel()
	mockStore := &mock.MetadataStore{}
	mds := transaction.NewMetadataService(mockStore, "net", "ch")
	ctx := t.Context()

	digestA := []byte{0xAA}
	mappingA := driver.TransientMap{"field_mapping|x": []byte("preimage-A")}
	mockStore.PutMetadataReturns(nil)
	require.NoError(t, mds.PutFieldMapping(ctx, "ns", "k", digestA, mappingA))

	// Put targets a (net, ch) key and stores the mapping value verbatim.
	_, keyA, valA := mockStore.PutMetadataArgsForCall(0)
	require.Equal(t, "net", keyA.Network)
	require.Equal(t, "ch", keyA.Channel)
	require.Equal(t, mappingA, valA)

	// A different digest for the same (ns,key) must map to a distinct KVS key
	// (overwrite-safe: the old preimage stays resolvable under its own digest).
	digestB := []byte{0xBB}
	mappingB := driver.TransientMap{"field_mapping|x": []byte("preimage-B")}
	require.NoError(t, mds.PutFieldMapping(ctx, "ns", "k", digestB, mappingB))
	_, keyB, _ := mockStore.PutMetadataArgsForCall(1)
	require.NotEqual(t, keyA.TxID, keyB.TxID, "different digests must map to different KVS keys")

	// GetFieldMapping returns whatever the store holds for that digest.
	mockStore.GetMetadataReturns(mappingA, nil)
	got, err := mds.GetFieldMapping(ctx, "ns", "k", digestA)
	require.NoError(t, err)
	require.Equal(t, mappingA, got)
}

func TestEnvelopeService(t *testing.T) {
	t.Parallel()
	mockStore := &mock.EnvelopeStore{}
	envs := transaction.NewEnvelopeService(mockStore, "network", "channel")

	ctx := t.Context()

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

func TestEndorseTransactionService(t *testing.T) {
	t.Parallel()
	mockStore := &mock.EndorseTxStore{}
	ets := transaction.NewEndorseTransactionService(mockStore, "network", "channel")

	ctx := t.Context()

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
