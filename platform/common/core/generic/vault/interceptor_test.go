/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault/fake"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
)

func TestConcurrency(t *testing.T) {
	t.Parallel()
	qe := fake.NewQE()
	idsr := fake.TxStatusStore{}

	i := newVaultInterceptor(logging.MustGetLogger(), context.Background(), EmptyRWSet(), qe, idsr, "1")
	s, err := i.GetState("ns", "key")
	require.NoError(t, err)
	require.Equal(t, qe.State.Raw, s, "with no opts, getstate should return the FromStorage value (query executor)")

	md, err := i.GetStateMetadata("ns", "key")
	require.NoError(t, err)
	require.Equal(t, qe.Metadata, md, "with no opts, GetStateMetadata should return the FromStorage value (query executor)")

	s, err = i.GetState("ns", "key", driver.FromBoth)
	require.NoError(t, err)
	require.Equal(t, qe.State.Raw, s, "FromBoth should fallback to FromStorage with empty rwset")

	md, err = i.GetStateMetadata("ns", "key", driver.FromBoth)
	require.NoError(t, err)
	require.Equal(t, qe.Metadata, md, "FromBoth should fallback to FromStorage with empty rwset")

	s, err = i.GetState("ns", "key", driver.FromIntermediate)
	require.NoError(t, err)
	require.Equal(t, []byte(nil), s, "FromIntermediate should return empty result from empty rwset")

	md, err = i.GetStateMetadata("ns", "key", driver.FromIntermediate)
	require.NoError(t, err)
	require.Nil(t, md, "FromIntermediate should return empty result from empty rwset")

	// Done in parallel
	wg := sync.WaitGroup{}
	wg.Add(3)
	f := func() {
		i.Done()
		wg.Done()
	}
	go f()
	go f()
	go f()
	wg.Wait()

	_, err = i.GetState("ns", "key")
	require.Error(t, err, "this instance was closed")
}

func TestAddReadAt(t *testing.T) {
	t.Parallel()
	qe := fake.QE{}
	idsr := fake.TxStatusStore{}
	i := newVaultInterceptor(logging.MustGetLogger(), context.Background(), EmptyRWSet(), qe, idsr, "1")

	require.NoError(t, i.AddReadAt("ns", "key", []byte("version")))
	require.Len(t, i.RWs().Reads, 1)
	require.Equal(t, []byte("version"), i.RWs().Reads["ns"]["key"])
}

// failingQE is a VersionedQueryExecutor whose every call fails, so the error
// paths the passing fake cannot reach become testable.
type failingQE struct {
	doneErr bool
}

func (failingQE) GetStateMetadata(context.Context, driver.Namespace, driver.PKey) (driver.Metadata, driver.RawVersion, error) {
	return nil, nil, errors.New("query executor unavailable")
}

func (failingQE) GetState(context.Context, driver.Namespace, driver.PKey) (*driver.VaultRead, error) {
	return nil, errors.New("query executor unavailable")
}

func (q failingQE) Done() error {
	if q.doneErr {
		return errors.New("close failed")
	}
	return nil
}

// staleQE returns a version that never matches what the read set recorded, so
// the version-comparison branches are reachable.
type staleQE struct{}

func (staleQE) GetStateMetadata(context.Context, driver.Namespace, driver.PKey) (driver.Metadata, driver.RawVersion, error) {
	return map[string][]byte{"md": []byte("meta")}, []byte("stale-version"), nil
}

func (staleQE) GetState(_ context.Context, _ driver.Namespace, pkey driver.PKey) (*driver.VaultRead, error) {
	return &driver.VaultRead{Key: pkey, Raw: []byte("raw"), Version: []byte("stale-version")}, nil
}

func (staleQE) Done() error { return nil }

// nilStateQE reports a key that is absent from storage.
type nilStateQE struct{}

func (nilStateQE) GetStateMetadata(context.Context, driver.Namespace, driver.PKey) (driver.Metadata, driver.RawVersion, error) {
	return nil, nil, nil
}

func (nilStateQE) GetState(context.Context, driver.Namespace, driver.PKey) (*driver.VaultRead, error) {
	return nil, nil
}

func (nilStateQE) Done() error { return nil }

func newTestInterceptor(qe VersionedQueryExecutor) *Interceptor[ValidationCode] {
	return NewInterceptor[ValidationCode](
		logging.MustGetLogger(),
		context.Background(),
		EmptyRWSet(),
		qe,
		fake.TxStatusStore{},
		"tx1",
		VCProvider,
		&marshaller{},
		&BlockTxIndexVersionComparator{},
	)
}

// TestInterceptorClosedRejectsCalls checks every entry point that guards on
// IsClosed refuses once Done has run.
func TestInterceptorClosedRejectsCalls(t *testing.T) {
	t.Parallel()

	i := newTestInterceptor(fake.NewQE())
	i.Done()
	require.True(t, i.IsClosed())

	require.ErrorContains(t, i.Clear("ns"), "this instance was closed")

	_, err := i.GetReadKeyAt("ns", 0)
	require.ErrorContains(t, err, "this instance was closed")

	_, _, err = i.GetReadAt("ns", 0)
	require.ErrorContains(t, err, "this instance was closed")

	errs := i.SetStateMetadatas("ns", map[driver.PKey]driver.Metadata{"k1": {"md": []byte("v")}})
	require.Len(t, errs, 1)
	require.ErrorContains(t, errs["k1"], "this instance was closed")
}

// TestInterceptorClear empties every namespace-scoped set while the
// interceptor is open.
func TestInterceptorClear(t *testing.T) {
	t.Parallel()

	i := newTestInterceptor(fake.NewQE())
	require.NoError(t, i.SetState("ns", "k1", []byte("v1")))
	require.NoError(t, i.SetStateMetadata("ns", "k1", map[string][]byte{"md": []byte("m")}))
	require.NoError(t, i.AddReadAt("ns", "k1", nil))

	require.NoError(t, i.Clear("ns"))

	require.False(t, i.rws.WriteSet.In("ns", "k1"))
	require.False(t, i.rws.MetaWriteSet.In("ns", "k1"))
	_, in := i.rws.ReadSet.Get("ns", "k1")
	require.False(t, in)
}

// TestInterceptorGetReadKeyAt covers both branches while open.
func TestInterceptorGetReadKeyAt(t *testing.T) {
	t.Parallel()

	i := newTestInterceptor(fake.NewQE())
	require.NoError(t, i.AddReadAt("ns", "k1", nil))

	key, err := i.GetReadKeyAt("ns", 0)
	require.NoError(t, err)
	require.Equal(t, "k1", key)

	_, err = i.GetReadKeyAt("ns", 1)
	require.ErrorContains(t, err, "no read at position 1 for namespace ns")
}

// TestInterceptorSetStateMetadatas applies every entry and reports no errors
// on the happy path.
func TestInterceptorSetStateMetadatas(t *testing.T) {
	t.Parallel()

	i := newTestInterceptor(fake.NewQE())

	errs := i.SetStateMetadatas("ns", map[driver.PKey]driver.Metadata{
		"k1": {"md": []byte("v1")},
		"k2": {"md": []byte("v2")},
	})
	require.Empty(t, errs)
	require.True(t, i.rws.MetaWriteSet.In("ns", "k1"))
	require.True(t, i.rws.MetaWriteSet.In("ns", "k2"))
}

// TestInterceptorGetStateOptErrors covers the option-validation branches
// shared by GetState and GetStateMetadata.
func TestInterceptorGetStateOptErrors(t *testing.T) {
	t.Parallel()

	i := newTestInterceptor(fake.NewQE())

	_, err := i.GetState("ns", "k1", driver.FromStorage, driver.FromIntermediate)
	require.ErrorContains(t, err, "a single getoption is supported")

	_, err = i.GetStateMetadata("ns", "k1", driver.FromStorage, driver.FromIntermediate)
	require.ErrorContains(t, err, "a single getoption is supported")

	_, err = i.GetState("ns", "k1", driver.GetStateOpt(99))
	require.ErrorContains(t, err, "invalid get option")

	_, err = i.GetStateMetadata("ns", "k1", driver.GetStateOpt(99))
	require.ErrorContains(t, err, "invalid get option")
}

// TestInterceptorQueryExecutorErrors checks failures from the query executor
// propagate rather than being swallowed.
func TestInterceptorQueryExecutorErrors(t *testing.T) {
	t.Parallel()

	i := newTestInterceptor(failingQE{})
	// IsValid only consults the query executor for keys already in the read
	// set, so record one before asserting it surfaces the failure.
	require.NoError(t, i.AddReadAt("ns", "k1", []byte("recorded-version")))

	_, err := i.GetState("ns", "k1")
	require.ErrorContains(t, err, "query executor unavailable")

	_, err = i.GetStateMetadata("ns", "k1")
	require.ErrorContains(t, err, "query executor unavailable")

	_, err = i.GetDirectState("ns", "k1")
	require.ErrorContains(t, err, "query executor unavailable")

	require.ErrorContains(t, i.IsValid(), "query executor unavailable")

	// GetReadAt resolves the key from the read set and then reads through to
	// storage, so a failing executor surfaces on the second step.
	_, _, err = i.GetReadAt("ns", 0)
	require.ErrorContains(t, err, "query executor unavailable")
}

// TestInterceptorGetDirectState returns the raw value straight from storage,
// bypassing the read set.
func TestInterceptorGetDirectState(t *testing.T) {
	t.Parallel()

	i := newTestInterceptor(fake.NewQE())

	val, err := i.GetDirectState("ns", "k1")
	require.NoError(t, err)
	require.Equal(t, []byte("raw"), val)

	_, in := i.rws.ReadSet.Get("ns", "k1")
	require.False(t, in, "a direct read must not be recorded in the read set")
}

// TestInterceptorVersionMismatch checks a read recorded at one version and
// then re-read at another is rejected rather than silently accepted.
func TestInterceptorVersionMismatch(t *testing.T) {
	t.Parallel()

	i := newTestInterceptor(staleQE{})
	require.NoError(t, i.AddReadAt("ns", "k1", []byte("recorded-version")))

	_, err := i.GetState("ns", "k1")
	require.ErrorContains(t, err, "invalid read")

	_, err = i.GetStateMetadata("ns", "k1")
	require.ErrorContains(t, err, "invalid metadata read")

	require.ErrorContains(t, i.IsValid(), "invalid read")
}

// TestInterceptorGetStateMissingKey checks an absent key reads as empty and
// still records the read.
func TestInterceptorGetStateMissingKey(t *testing.T) {
	t.Parallel()

	i := newTestInterceptor(nilStateQE{})

	val, err := i.GetState("ns", "absent")
	require.NoError(t, err)
	require.Empty(t, val)

	_, in := i.rws.ReadSet.Get("ns", "absent")
	require.True(t, in, "a miss is still a read and must be recorded")
}

// TestInterceptorIsValidWithoutQueryExecutor treats a write-only interceptor
// as trivially valid.
func TestInterceptorIsValidWithoutQueryExecutor(t *testing.T) {
	t.Parallel()

	i := newTestInterceptor(nil)
	require.NoError(t, i.IsValid())

	_, err := i.GetState("ns", "k1")
	require.ErrorContains(t, err, "this instance is write only")

	_, err = i.GetStateMetadata("ns", "k1")
	require.ErrorContains(t, err, "this instance is write only")
}

// TestInterceptorEquals covers comparison against another interceptor, against
// an inspector, and against an unsupported type.
func TestInterceptorEquals(t *testing.T) {
	t.Parallel()

	build := func() *Interceptor[ValidationCode] {
		i := newTestInterceptor(fake.NewQE())
		require.NoError(t, i.SetState("ns", "k1", []byte("v1")))
		return i
	}

	a, b := build(), build()
	require.NoError(t, a.Equals(b))

	other := newTestInterceptor(fake.NewQE())
	require.NoError(t, other.SetState("ns", "k1", []byte("different")))
	require.ErrorContains(t, a.Equals(other), "writes do not match")

	inspector := &Inspector{Rws: EmptyRWSet()}
	require.NoError(t, inspector.Rws.WriteSet.Add("ns", "k1", []byte("v1")))
	require.NoError(t, a.Equals(inspector))

	require.ErrorContains(t, a.Equals("not an rwset"), "cannot compare to the passed value")

	// A mismatch in any of the three sets is reported against that set.
	readMismatch := newTestInterceptor(fake.NewQE())
	require.NoError(t, readMismatch.SetState("ns", "k1", []byte("v1")))
	require.NoError(t, readMismatch.AddReadAt("ns", "k1", []byte("version")))
	require.ErrorContains(t, a.Equals(readMismatch), "reads do not match")

	metaMismatch := newTestInterceptor(fake.NewQE())
	require.NoError(t, metaMismatch.SetState("ns", "k1", []byte("v1")))
	require.NoError(t, metaMismatch.SetStateMetadata("ns", "k1", map[string][]byte{"md": []byte("m")}))
	require.ErrorContains(t, a.Equals(metaMismatch), "meta writes do not match")

	// The same three comparisons run against an Inspector.
	inspectorReads := &Inspector{Rws: EmptyRWSet()}
	require.NoError(t, inspectorReads.Rws.WriteSet.Add("ns", "k1", []byte("v1")))
	inspectorReads.Rws.ReadSet.Add("ns", "k1", []byte("version"))
	require.ErrorContains(t, a.Equals(inspectorReads), "reads do not match")

	inspectorWrites := &Inspector{Rws: EmptyRWSet()}
	require.NoError(t, inspectorWrites.Rws.WriteSet.Add("ns", "k1", []byte("different")))
	require.ErrorContains(t, a.Equals(inspectorWrites), "writes do not match")

	inspectorMeta := &Inspector{Rws: EmptyRWSet()}
	require.NoError(t, inspectorMeta.Rws.WriteSet.Add("ns", "k1", []byte("v1")))
	require.NoError(t, inspectorMeta.Rws.MetaWriteSet.Add("ns", "k1", map[string][]byte{"md": []byte("m")}))
	require.ErrorContains(t, a.Equals(inspectorMeta), "meta writes do not match")
}

// TestInterceptorReopen checks Done then Reopen restores a usable interceptor,
// and that reopening an open one is rejected.
func TestInterceptorReopen(t *testing.T) {
	t.Parallel()

	i := newTestInterceptor(fake.NewQE())
	require.ErrorContains(t, i.Reopen(fake.NewQE()), "already open")

	i.Done()
	require.True(t, i.IsClosed())

	require.NoError(t, i.Reopen(fake.NewQE()))
	require.False(t, i.IsClosed())

	_, err := i.GetState("ns", "k1")
	require.NoError(t, err, "a reopened interceptor must serve reads again")
}

// TestInterceptorDoneIsIdempotent checks repeated Done calls are safe, and
// that a failing query executor close is logged rather than propagated.
func TestInterceptorDoneIsIdempotent(t *testing.T) {
	t.Parallel()

	i := newTestInterceptor(failingQE{doneErr: true})

	require.NotPanics(t, i.Done)
	require.True(t, i.IsClosed())

	require.NotPanics(t, i.Done)
	require.True(t, i.IsClosed())
}
