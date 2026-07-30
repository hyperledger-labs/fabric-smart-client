/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault_test

import (
	"context"
	"testing"

	"github.com/hyperledger/fabric-x-common/api/applicationpb"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/encoding/protowire"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
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

func (m *mockQueryService) GetTransactionStatuses(txIDs []string) (map[string]int32, error) {
	out := make(map[string]int32, len(txIDs))
	for _, txID := range txIDs {
		if status, ok := m.txStatuses[txID]; ok {
			out[txID] = status
		}
	}
	return out, nil
}

func (m *mockQueryService) GetConfigTransaction() (*queryservice.ConfigTransactionInfo, error) {
	return nil, nil
}

func (m *mockQueryService) GetNamespacePolicies() (*applicationpb.NamespacePolicies, error) {
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

// TestVaultX_Status_Unknown asserts that Status agrees with Statuses on the "committer doesn't know
// this tx" case: it reports Unknown ("not final yet") without an error, rather than surfacing the
// unknown-tx condition as a failure to a caller polling an in-flight transaction.
func TestVaultX_Status_Unknown(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	// tx1 intentionally left unset => committer does not know it yet.

	v := vault.NewVault(qs)
	ctx := context.Background()

	code, msg, err := v.Status(ctx, "tx1")
	require.NoError(t, err)
	require.Equal(t, fdriver.Unknown, code)
	require.Empty(t, msg)
}

func TestVaultX_Statuses(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	qs.setTxStatus("tx1", 1) // COMMITTED
	qs.setTxStatus("tx2", 0) // UNSPECIFIED
	// tx3 is intentionally left unset: the batched query omits transactions the committer does not
	// know, and Statuses must report those as Unknown ("not final yet") in the right position.

	v := vault.NewVault(qs)
	ctx := context.Background()

	statuses, err := v.Statuses(ctx, "tx1", "tx3", "tx2")
	require.NoError(t, err)
	require.Len(t, statuses, 3)

	// Order matches the input, not the (unordered) map returned by the batched query.
	require.Equal(t, driver.TxID("tx1"), statuses[0].TxID)
	require.Equal(t, fdriver.Valid, statuses[0].ValidationCode)

	require.Equal(t, driver.TxID("tx3"), statuses[1].TxID)
	require.Equal(t, fdriver.Unknown, statuses[1].ValidationCode)

	require.Equal(t, driver.TxID("tx2"), statuses[2].TxID)
	require.Equal(t, fdriver.Unknown, statuses[2].ValidationCode)
}

// The commit-pipeline methods (SetDiscarded/DiscardTx/CommitTX/Match/RWSExists) are unreachable in
// the fabricx wiring, which has no local commit pipeline. They panic if called, so that wiring the
// vault into a generic committer by mistake fails loudly rather than silently misbehaving.

func TestVaultX_SetDiscarded(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	v := vault.NewVault(qs)
	ctx := context.Background()

	require.PanicsWithValue(t,
		"fabricx vault: SetDiscarded called; fabricx has no local commit pipeline",
		func() { _ = v.SetDiscarded(ctx, "tx1", "test error") })
}

func TestVaultX_DiscardTx(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	v := vault.NewVault(qs)
	ctx := context.Background()

	require.PanicsWithValue(t,
		"fabricx vault: DiscardTx called; fabricx has no local commit pipeline",
		func() { _ = v.DiscardTx(ctx, "tx1", "discard reason") })
}

func TestVaultX_CommitTX(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	v := vault.NewVault(qs)
	ctx := context.Background()

	require.PanicsWithValue(t,
		"fabricx vault: CommitTX called; fabricx has no local commit pipeline",
		func() { _ = v.CommitTX(ctx, "tx1", 10, 5) })
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

	// RWSExists has no error channel, so returning a value would be a misleading answer; it panics.
	require.PanicsWithValue(t,
		"fabricx vault: RWSExists called; fabricx has no local commit pipeline",
		func() { _ = v.RWSExists(ctx, "tx1") })
}

func TestVaultX_Match(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	v := vault.NewVault(qs)
	ctx := context.Background()

	// The vault retains no RWSet to match against, and Match is unreachable in the fabricx wiring,
	// so it panics — including for a txID whose RWSet was just created and marshalled.
	rws, err := v.NewRWSet(ctx, "tx1")
	require.NoError(t, err)

	err = rws.SetState("ns1", "key1", []byte("value1"))
	require.NoError(t, err)

	bytes, err := rws.Bytes()
	require.NoError(t, err)

	require.PanicsWithValue(t,
		"fabricx vault: Match called; fabricx has no local commit pipeline",
		func() { _ = v.Match(ctx, "tx1", bytes) })
}

func TestVaultX_Close(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	v := vault.NewVault(qs)
	ctx := context.Background()

	// The vault holds no per-transaction state, so Close is a no-op that always succeeds.
	_, err := v.NewRWSet(ctx, "tx1")
	require.NoError(t, err)

	err = v.Close()
	require.NoError(t, err)
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

// TestRWSet_ConcurrentBytes exercises many goroutines calling Bytes() on different RWSets created
// from the same vault. All those wrappers share the vault's single Marshaller, so before Bytes()
// stopped writing the marshaller's namespace-info through shared state, this raced. Run under
// -race, it fails on the old code and locks the fix in.
func TestRWSet_ConcurrentBytes(t *testing.T) {
	t.Parallel()
	qs := newMockQueryService()
	// Give the namespaces a non-default _meta version so Bytes() builds a populated nsInfo map.
	qs.setState("_meta", "ns1", nil, 7)
	qs.setState("_meta", "ns2", nil, 9)

	v := vault.NewVault(qs)
	ctx := context.Background()

	const goroutines = 16
	// Assertions must not run off the test goroutine (require.* calls t.FailNow, which is only
	// valid from the goroutine running the test), so errors are collected and checked below.
	var eg errgroup.Group
	for g := range goroutines {
		eg.Go(func() error {
			// Each goroutine gets its own RWSet, but they all share v.marshaller.
			rws, err := v.NewRWSet(ctx, driver.TxID("tx"))
			if err != nil {
				return errors.Wrapf(err, "goroutine %d: NewRWSet", g)
			}
			// Vary the namespace per goroutine so the nsInfo maps differ between concurrent calls.
			ns := driver.Namespace("ns1")
			if g%2 == 0 {
				ns = "ns2"
			}
			if err := rws.SetState(ns, "key", []byte("value")); err != nil {
				return errors.Wrapf(err, "goroutine %d: SetState", g)
			}
			if _, err := rws.Bytes(); err != nil {
				return errors.Wrapf(err, "goroutine %d: Bytes", g)
			}
			return nil
		})
	}
	require.NoError(t, eg.Wait())
}
