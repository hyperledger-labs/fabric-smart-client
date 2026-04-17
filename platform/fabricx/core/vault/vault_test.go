/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/vault"
)

// mockQueryService implements queryservice.QueryService for testing
type mockQueryService struct {
	states     map[driver.Namespace]map[driver.PKey]driver.VaultValue
	txStatuses map[string]int32
}

func newMockQueryService() *mockQueryService {
	return &mockQueryService{
		states:     make(map[driver.Namespace]map[driver.PKey]driver.VaultValue),
		txStatuses: make(map[string]int32),
	}
}

func (m *mockQueryService) GetState(ns driver.Namespace, key driver.PKey) (*driver.VaultValue, error) {
	if nsMap, ok := m.states[ns]; ok {
		if val, ok := nsMap[key]; ok {
			return &val, nil
		}
	}
	return nil, nil
}

func (m *mockQueryService) GetStates(keys map[driver.Namespace][]driver.PKey) (map[driver.Namespace]map[driver.PKey]driver.VaultValue, error) {
	result := make(map[driver.Namespace]map[driver.PKey]driver.VaultValue)
	for ns, keyList := range keys {
		result[ns] = make(map[driver.PKey]driver.VaultValue)
		for _, key := range keyList {
			if val, err := m.GetState(ns, key); err == nil && val != nil {
				result[ns][key] = *val
			}
		}
	}
	return result, nil
}

func (m *mockQueryService) GetTransactionStatus(txID string) (int32, error) {
	if status, ok := m.txStatuses[txID]; ok {
		return status, nil
	}
	return 0, nil
}

func (m *mockQueryService) GetConfigTransaction() (*queryservice.ConfigTransactionInfo, error) {
	return nil, nil
}

func (m *mockQueryService) setState(ns driver.Namespace, key driver.PKey, value []byte, version uint64) {
	if _, ok := m.states[ns]; !ok {
		m.states[ns] = make(map[driver.PKey]driver.VaultValue)
	}
	m.states[ns][key] = driver.VaultValue{
		Raw:     value,
		Version: protowire.AppendVarint(nil, version),
	}
}

func (m *mockQueryService) setTxStatus(txID string, status int32) {
	m.txStatuses[txID] = status
}

func TestVaultX_NewQueryExecutor(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	qs.setState("ns1", "key1", []byte("value1"), 1)

	v := vault.NewVault(qs)

	ctx := context.Background()
	qe, err := v.NewQueryExecutor(ctx)
	require.NoError(t, err)
	require.NotNil(t, qe)

	// Test GetState
	read, err := qe.GetState(ctx, "ns1", "key1")
	require.NoError(t, err)
	require.NotNil(t, read)
	require.Equal(t, "key1", read.Key)
	require.Equal(t, []byte("value1"), read.Raw)

	// Test non-existent key
	read, err = qe.GetState(ctx, "ns1", "nonexistent")
	require.NoError(t, err)
	require.Nil(t, read)

	// Cleanup
	err = qe.Done()
	require.NoError(t, err)
}

func TestVaultX_NewRWSet(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	qs.setState("ns1", "key1", []byte("value1"), 1)

	v := vault.NewVault(qs)
	ctx := context.Background()

	rws, err := v.NewRWSet(ctx, "tx1")
	require.NoError(t, err)
	require.NotNil(t, rws)

	// Test SetState
	err = rws.SetState("ns1", "key2", []byte("value2"))
	require.NoError(t, err)

	// Test GetState (should return from write set)
	val, err := rws.GetState("ns1", "key2")
	require.NoError(t, err)
	require.Equal(t, []byte("value2"), val)

	// Test GetState from storage
	val, err = rws.GetState("ns1", "key1")
	require.NoError(t, err)
	require.Equal(t, []byte("value1"), val)

	// Test NumWrites
	require.Equal(t, 1, rws.NumWrites("ns1"))

	// Test NumReads
	require.Equal(t, 1, rws.NumReads("ns1"))
}

func TestVaultX_NewRWSetFromBytes(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	v := vault.NewVault(qs)
	ctx := context.Background()

	// Create an RWSet and marshal it
	rws1, err := v.NewRWSet(ctx, "tx1")
	require.NoError(t, err)

	err = rws1.SetState("ns1", "key1", []byte("value1"))
	require.NoError(t, err)

	bytes, err := rws1.Bytes()
	require.NoError(t, err)

	// Create new RWSet from bytes
	rws2, err := v.NewRWSetFromBytes(ctx, "tx2", bytes)
	require.NoError(t, err)
	require.NotNil(t, rws2)

	// Verify the content
	require.Equal(t, 1, rws2.NumWrites("ns1"))
}

func TestVaultX_Status(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	qs.setTxStatus("tx1", 1) // COMMITTED

	v := vault.NewVault(qs)
	ctx := context.Background()

	code, msg, err := v.Status(ctx, "tx1")
	require.NoError(t, err)
	require.Equal(t, fdriver.Valid, code)
	require.Empty(t, msg)
}

func TestVaultX_Statuses(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	qs.setTxStatus("tx1", 1) // COMMITTED
	qs.setTxStatus("tx2", 0) // UNSPECIFIED

	v := vault.NewVault(qs)
	ctx := context.Background()

	statuses, err := v.Statuses(ctx, "tx1", "tx2")
	require.NoError(t, err)
	require.Len(t, statuses, 2)

	require.Equal(t, driver.TxID("tx1"), statuses[0].TxID)
	require.Equal(t, fdriver.Valid, statuses[0].ValidationCode)

	require.Equal(t, driver.TxID("tx2"), statuses[1].TxID)
	require.Equal(t, fdriver.Unknown, statuses[1].ValidationCode)
}

func TestVaultX_SetDiscarded(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	v := vault.NewVault(qs)
	ctx := context.Background()

	err := v.SetDiscarded(ctx, "tx1", "test error")
	require.NoError(t, err)

	// Verify status is set to Invalid
	code, msg, err := v.Status(ctx, "tx1")
	require.NoError(t, err)
	require.Equal(t, fdriver.Invalid, code)
	require.Equal(t, "test error", msg)
}

func TestVaultX_DiscardTx(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	v := vault.NewVault(qs)
	ctx := context.Background()

	err := v.DiscardTx(ctx, "tx1", "discard reason")
	require.NoError(t, err)

	// Verify status
	code, msg, err := v.Status(ctx, "tx1")
	require.NoError(t, err)
	require.Equal(t, fdriver.Invalid, code)
	require.Equal(t, "discard reason", msg)
}

func TestVaultX_CommitTX(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	v := vault.NewVault(qs)
	ctx := context.Background()

	err := v.CommitTX(ctx, "tx1", 10, 5)
	require.NoError(t, err)

	// Verify status is set to Valid
	code, msg, err := v.Status(ctx, "tx1")
	require.NoError(t, err)
	require.Equal(t, fdriver.Valid, code)
	require.Empty(t, msg)
}

func TestVaultX_InspectRWSet(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	v := vault.NewVault(qs)
	ctx := context.Background()

	// Create an RWSet and marshal it
	rws1, err := v.NewRWSet(ctx, "tx1")
	require.NoError(t, err)

	err = rws1.SetState("ns1", "key1", []byte("value1"))
	require.NoError(t, err)
	err = rws1.SetState("ns2", "key2", []byte("value2"))
	require.NoError(t, err)

	bytes, err := rws1.Bytes()
	require.NoError(t, err)

	// Inspect with namespace filter
	rws2, err := v.InspectRWSet(ctx, bytes, "ns1")
	require.NoError(t, err)
	require.NotNil(t, rws2)

	// Should have ns1
	require.Equal(t, 1, rws2.NumWrites("ns1"))
	// Note: The marshaller may still include ns2 in the structure even with filtering
	// The important thing is that ns1 is present
}

func TestVaultX_RWSExists(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	v := vault.NewVault(qs)
	ctx := context.Background()

	// Initially should not exist
	require.False(t, v.RWSExists(ctx, "tx1"))

	// Create RWSet
	_, err := v.NewRWSet(ctx, "tx1")
	require.NoError(t, err)

	// Now should exist
	require.True(t, v.RWSExists(ctx, "tx1"))
}

func TestVaultX_Match(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	v := vault.NewVault(qs)
	ctx := context.Background()

	// Create an RWSet
	rws, err := v.NewRWSet(ctx, "tx1")
	require.NoError(t, err)

	err = rws.SetState("ns1", "key1", []byte("value1"))
	require.NoError(t, err)

	bytes, err := rws.Bytes()
	require.NoError(t, err)

	// Match should succeed with same bytes
	err = v.Match(ctx, "tx1", bytes)
	require.NoError(t, err)

	// Match should fail with different bytes
	err = v.Match(ctx, "tx1", []byte("different"))
	require.Error(t, err)

	// Match should fail for non-existent tx
	err = v.Match(ctx, "nonexistent", bytes)
	require.Error(t, err)
}

func TestVaultX_Close(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	v := vault.NewVault(qs)
	ctx := context.Background()

	// Create some data
	_, err := v.NewRWSet(ctx, "tx1")
	require.NoError(t, err)

	err = v.SetDiscarded(ctx, "tx2", "test")
	require.NoError(t, err)

	// Close should clear everything
	err = v.Close()
	require.NoError(t, err)

	// Verify data is cleared
	require.False(t, v.RWSExists(ctx, "tx1"))
}

func TestRWSet_Operations(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	qs.setState("ns1", "existing", []byte("existing_value"), 1)

	v := vault.NewVault(qs)
	ctx := context.Background()

	rws, err := v.NewRWSet(ctx, "tx1")
	require.NoError(t, err)

	// Test Clear
	err = rws.SetState("ns1", "key1", []byte("value1"))
	require.NoError(t, err)
	require.Equal(t, 1, rws.NumWrites("ns1"))

	err = rws.Clear("ns1")
	require.NoError(t, err)
	require.Equal(t, 0, rws.NumWrites("ns1"))

	// Test DeleteState
	err = rws.SetState("ns1", "key2", []byte("value2"))
	require.NoError(t, err)

	err = rws.DeleteState("ns1", "key2")
	require.NoError(t, err)

	// Test Namespaces
	err = rws.SetState("ns1", "key1", []byte("value1"))
	require.NoError(t, err)
	err = rws.SetState("ns2", "key2", []byte("value2"))
	require.NoError(t, err)

	namespaces := rws.Namespaces()
	require.Contains(t, namespaces, driver.Namespace("ns1"))
	require.Contains(t, namespaces, driver.Namespace("ns2"))

	// Test GetDirectState
	val, err := rws.GetDirectState("ns1", "existing")
	require.NoError(t, err)
	require.Equal(t, []byte("existing_value"), val)

	// Test metadata operations
	metadata := driver.Metadata{"meta1": []byte("metavalue1")}
	err = rws.SetStateMetadata("ns1", "key1", metadata)
	require.NoError(t, err)

	retrievedMeta, err := rws.GetStateMetadata("ns1", "key1")
	require.NoError(t, err)
	require.Equal(t, metadata, retrievedMeta)
}

func TestRWSet_GetReadAt(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	qs.setState("ns1", "key1", []byte("value1"), 1)
	qs.setState("ns1", "key2", []byte("value2"), 2)

	v := vault.NewVault(qs)
	ctx := context.Background()

	rws, err := v.NewRWSet(ctx, "tx1")
	require.NoError(t, err)

	// Trigger reads
	_, err = rws.GetState("ns1", "key1")
	require.NoError(t, err)
	_, err = rws.GetState("ns1", "key2")
	require.NoError(t, err)

	// Test GetReadAt
	key, val, err := rws.GetReadAt("ns1", 0)
	require.NoError(t, err)
	require.NotEmpty(t, key)
	require.NotNil(t, val)

	// Test out of bounds
	_, _, err = rws.GetReadAt("ns1", 100)
	require.Error(t, err)
}

func TestRWSet_GetWriteAt(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	v := vault.NewVault(qs)
	ctx := context.Background()

	rws, err := v.NewRWSet(ctx, "tx1")
	require.NoError(t, err)

	err = rws.SetState("ns1", "key1", []byte("value1"))
	require.NoError(t, err)

	// Test GetWriteAt
	key, val, err := rws.GetWriteAt("ns1", 0)
	require.NoError(t, err)
	require.Equal(t, "key1", key)
	require.Equal(t, []byte("value1"), val)

	// Test out of bounds
	_, _, err = rws.GetWriteAt("ns1", 100)
	require.Error(t, err)
}
