/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver"
	vault2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/storage/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
)

type testArtifactProvider struct {
	removeNils func([]driver2.VaultRead) []driver2.VaultRead
}

func (p *testArtifactProvider) RemoveNils(items []driver2.VaultRead) []driver2.VaultRead {
	return p.removeNils(items)
}

func (p *testArtifactProvider) NewCachedVault(ddb driver.VaultStore) (*Vault[ValidationCode], error) {
	vaultLogger := logging.MustGetLogger()
	return New[ValidationCode](
		vaultLogger,
		vault2.NewCachedVault(ddb, 100),
		VCProvider,
		newInterceptor,
		&populator{},
		&disabled.Provider{},
		&noop.TracerProvider{},
		&BlockTxIndexVersionBuilder{},
	), nil
}

func (p *testArtifactProvider) NewNonCachedVault(ddb driver.VaultStore) (*Vault[ValidationCode], error) {
	return New[ValidationCode](
		logging.MustGetLogger(),
		vault2.NewCachedVault(ddb, 0),
		VCProvider,
		newInterceptor,
		&populator{},
		&disabled.Provider{},
		&noop.TracerProvider{},
		&BlockTxIndexVersionBuilder{},
	), nil
}

func (p *testArtifactProvider) NewMarshaller() Marshaller {
	return &marshaller{}
}

func newInterceptor(
	logger Logger,
	ctx context.Context,
	rwSet ReadWriteSet,
	qe VersionedQueryExecutor,
	txidStore TxStatusStore,
	txid driver2.TxID,
) TxInterceptor {
	return NewInterceptor[ValidationCode](
		logger,
		ctx,
		rwSet,
		qe,
		txidStore,
		txid,
		VCProvider,
		&marshaller{},
		&BlockTxIndexVersionComparator{},
	)
}

type populator struct {
	marshaller marshaller
}

func (p *populator) Populate(rwsetBytes []byte, namespaces ...driver2.Namespace) (ReadWriteSet, error) {
	rwSet := EmptyRWSet()
	if err := p.marshaller.Append(&rwSet, rwsetBytes, namespaces...); err != nil {
		return ReadWriteSet{}, err
	}
	return rwSet, nil
}

type marshaller struct{}

func (m *marshaller) Marshal(txID string, rws *ReadWriteSet) ([]byte, error) {
	return json.Marshal(rws)
}

func (m *marshaller) Append(destination *ReadWriteSet, raw []byte, nss ...string) error {
	source := &ReadWriteSet{}
	err := json.Unmarshal(raw, source)
	if err != nil {
		return errors.Wrapf(err, "provided invalid read-write set bytes, unmarshal failed")
	}

	namespaces := collections.NewSet(nss...)

	// readset
	for ns, reads := range source.Reads {
		if len(nss) != 0 && !namespaces.Contains(ns) {
			continue
		}
		for s, position := range reads {
			v, in := destination.ReadSet.Get(ns, s)
			if in && !Equal(position, v) {
				return errors.Errorf("invalid read [%s:%s]: previous value returned at version [%v], current value at version [%v]", ns, s, position, v)
			}
			destination.ReadSet.Add(ns, s, position)
		}
	}
	destination.OrderedReads = source.OrderedReads

	// writeset
	for ns, writes := range source.Writes {
		if len(nss) != 0 && !namespaces.Contains(ns) {
			continue
		}
		for s, position := range writes {
			if destination.WriteSet.In(ns, s) {
				return errors.Errorf("duplicate write entry for key %s:%s", ns, s)
			}
			if err := destination.WriteSet.Add(ns, s, position); err != nil {
				return err
			}
		}
	}
	destination.OrderedWrites = source.OrderedWrites

	// meta writes
	for ns, writes := range source.MetaWrites {
		if len(nss) != 0 && !namespaces.Contains(ns) {
			continue
		}
		for s, position := range writes {
			if destination.MetaWriteSet.In(ns, s) {
				return errors.Errorf("duplicate metadata write entry for key %s:%s", ns, s)
			}
			if err := destination.MetaWriteSet.Add(ns, s, position); err != nil {
				return errors.Wrapf(err, "duplicate metadata write entry for key %s:%s", ns, s)
			}
		}
	}

	return nil
}

func TestMemory(t *testing.T) { //nolint:tparallel
	t.Parallel()
	artifactProvider := &testArtifactProvider{
		removeNils: func(items []driver2.VaultRead) []driver2.VaultRead { return items },
	}
	for _, c := range SingleDBCases { //nolint:paralleltest
		t.Run(c.Name, func(xt *testing.T) {
			ddb, err := vault2.OpenMemoryVault(c.Name)
			require.NoError(t, err)
			require.NotNil(t, ddb)
			defer utils.IgnoreErrorFunc(ddb.Close)
			c.Fn(xt, ddb, artifactProvider)
		})
	}

	for _, c := range DoubleDBCases { //nolint:paralleltest
		t.Run(c.Name, func(xt *testing.T) {
			db1, err := vault2.OpenMemoryVault(c.Name)
			require.NoError(t, err)
			db2, err := vault2.OpenMemoryVault(c.Name)
			require.NoError(t, err)
			defer utils.IgnoreErrorFunc(db1.Close)
			defer utils.IgnoreErrorFunc(db2.Close)
			c.Fn(xt, db1, db2, artifactProvider)
		})
	}
}

func TestSqlite(t *testing.T) { //nolint:tparallel
	t.Parallel()
	artifactProvider := &testArtifactProvider{
		removeNils: func(items []driver2.VaultRead) []driver2.VaultRead {
			return slices.DeleteFunc(items, func(e driver2.VaultRead) bool { return e.Raw == nil }) //nolint:govet
		},
	}

	for _, c := range SingleDBCases { //nolint:paralleltest
		t.Run(c.Name, func(xt *testing.T) {
			ddb, err := vault2.OpenSqliteVault("node1", t.TempDir())
			require.NoError(t, err)
			defer utils.IgnoreErrorFunc(ddb.Close)
			c.Fn(xt, ddb, artifactProvider)
		})
	}

	for _, c := range DoubleDBCases { //nolint:paralleltest
		t.Run(c.Name, func(xt *testing.T) {
			db1, err := vault2.OpenSqliteVault("node1", t.TempDir())
			require.NoError(t, err)
			db2, err := vault2.OpenSqliteVault("node2", t.TempDir())
			require.NoError(t, err)
			defer utils.IgnoreErrorFunc(db1.Close)
			defer utils.IgnoreErrorFunc(db2.Close)
			c.Fn(xt, db1, db2, artifactProvider)
		})
	}
}

func TestPostgres(t *testing.T) { //nolint:tparallel
	t.Parallel()
	artifactProvider := &testArtifactProvider{
		removeNils: func(items []driver2.VaultRead) []driver2.VaultRead {
			return slices.DeleteFunc(items, func(e driver2.VaultRead) bool { return e.Raw == nil }) //nolint:govet
		},
	}

	for _, c := range append(SingleDBCases, ReadCommittedDBCases...) { //nolint:paralleltest
		t.Run(c.Name, func(xt *testing.T) {
			ddb, terminate, err := vault2.OpenPostgresVault("common-sdk-node1")
			require.NoError(t, err)
			defer utils.IgnoreErrorFunc(ddb.Close)
			defer terminate()
			c.Fn(xt, ddb, artifactProvider)
		})
	}

	for _, c := range DoubleDBCases { //nolint:paralleltest
		t.Run(c.Name, func(xt *testing.T) {
			db1, terminate1, err := vault2.OpenPostgresVault("common-sdk-node1")
			require.NoError(t, err)
			db2, terminate2, err := vault2.OpenPostgresVault("common-sdk-node2")
			require.NoError(t, err)
			defer utils.IgnoreErrorFunc(db1.Close)
			defer utils.IgnoreErrorFunc(db2.Close)
			defer terminate1()
			defer terminate2()
			c.Fn(xt, db1, db2, artifactProvider)
		})
	}
}

// newMemoryVault builds a vault backed by an in-memory store, closed when the
// test finishes.
func newMemoryVault(t *testing.T) (*Vault[ValidationCode], driver.VaultStore) {
	t.Helper()

	ddb, err := vault2.OpenMemoryVault(t.Name())
	require.NoError(t, err)
	require.NotNil(t, ddb)
	t.Cleanup(func() { utils.IgnoreErrorFunc(ddb.Close) })

	provider := &testArtifactProvider{
		removeNils: func(items []driver2.VaultRead) []driver2.VaultRead { return items },
	}
	v, err := provider.NewNonCachedVault(ddb)
	require.NoError(t, err)

	return v, ddb
}

// TestVaultRWSExists reports whether an interceptor is currently registered
// for a transaction.
func TestVaultRWSExists(t *testing.T) { //nolint:paralleltest
	v, _ := newMemoryVault(t)
	ctx := t.Context()

	require.False(t, v.RWSExists(ctx, "tx1"), "no interceptor registered yet")

	_, err := v.NewRWSet(ctx, "tx1")
	require.NoError(t, err)
	require.True(t, v.RWSExists(ctx, "tx1"))

	require.False(t, v.RWSExists(ctx, "other"), "unrelated transactions are unaffected")

	// Unmapping is refused while the interceptor is still open.
	_, err = v.UnmapInterceptor("tx1")
	require.ErrorContains(t, err, "done has not been called")
	require.True(t, v.RWSExists(ctx, "tx1"))

	rws, err := v.NewRWSet(ctx, "tx2")
	require.NoError(t, err)
	rws.Done()

	unmapped, err := v.UnmapInterceptor("tx2")
	require.NoError(t, err)
	require.NotNil(t, unmapped)
	require.False(t, v.RWSExists(ctx, "tx2"), "unmapping a closed interceptor removes it")

	_, err = v.UnmapInterceptor("absent")
	require.ErrorContains(t, err, "could not be found")
}

// TestVaultNewRWSetWithIsolationLevel checks the explicit-isolation entry point
// registers an interceptor the same way NewRWSet does.
func TestVaultNewRWSetWithIsolationLevel(t *testing.T) { //nolint:paralleltest
	v, _ := newMemoryVault(t)
	ctx := t.Context()

	rws, err := v.NewRWSetWithIsolationLevel(ctx, "tx1", driver2.LevelDefault)
	require.NoError(t, err)
	require.NotNil(t, rws)
	require.True(t, v.RWSExists(ctx, "tx1"))

	_, err = v.NewRWSetWithIsolationLevel(ctx, "tx1", driver2.LevelDefault)
	require.Error(t, err, "a second rwset for the same transaction is rejected")
}

// TestVaultSetStatusAndStatuses round-trips validation codes through the store.
func TestVaultSetStatusAndStatuses(t *testing.T) { //nolint:paralleltest
	v, _ := newMemoryVault(t)
	ctx := t.Context()

	require.NoError(t, v.SetStatus(ctx, "tx1", valid))
	require.NoError(t, v.SetStatus(ctx, "tx2", invalid))

	code, _, err := v.Status(ctx, "tx1")
	require.NoError(t, err)
	require.Equal(t, valid, code)

	statuses, err := v.Statuses(ctx, "tx1", "tx2")
	require.NoError(t, err)
	require.Len(t, statuses, 2)

	byTxID := map[driver2.TxID]driver2.TxValidationStatus[ValidationCode]{}
	for _, s := range statuses {
		byTxID[s.TxID] = s
	}
	require.Equal(t, valid, byTxID["tx1"].ValidationCode)
	require.Equal(t, invalid, byTxID["tx2"].ValidationCode)
}

// TestVaultStatusesEmpty checks querying no transactions is not an error.
func TestVaultStatusesEmpty(t *testing.T) { //nolint:paralleltest
	v, _ := newMemoryVault(t)

	statuses, err := v.Statuses(t.Context())
	require.NoError(t, err)
	require.Empty(t, statuses)
}

// TestVaultSetDiscarded marks a transaction invalid and records the reason.
func TestVaultSetDiscarded(t *testing.T) { //nolint:paralleltest
	v, _ := newMemoryVault(t)
	ctx := t.Context()

	require.NoError(t, v.SetDiscarded(ctx, "tx1", "conflicting write"))

	code, message, err := v.Status(ctx, "tx1")
	require.NoError(t, err)
	require.Equal(t, invalid, code)
	require.Equal(t, "conflicting write", message)

	statuses, err := v.Statuses(ctx, "tx1")
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	require.Equal(t, "conflicting write", statuses[0].Message)
}

// TestVaultClose closes the underlying store.
func TestVaultClose(t *testing.T) { //nolint:paralleltest
	ddb, err := vault2.OpenMemoryVault(t.Name())
	require.NoError(t, err)

	provider := &testArtifactProvider{
		removeNils: func(items []driver2.VaultRead) []driver2.VaultRead { return items },
	}
	v, err := provider.NewNonCachedVault(ddb)
	require.NoError(t, err)

	require.NoError(t, v.Close())
}

var _ = context.Background
