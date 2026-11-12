/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer/fake"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	fvault "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/storage/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
)

// Replicas of a node share one vault store. A replica whose delivery lags behind still
// learns that a transaction it endorsed is final once another replica commits it: its
// IsFinal rereads the shared status while it waits, rather than a Busy it cached.
func TestIsFinalSeesCommitByAnotherReplica(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	store, err := fvault.OpenSqliteVault("node", t.TempDir())
	require.NoError(t, err)
	defer func() { _ = store.Close() }()
	newReplicaVault := func() *vault.Vault {
		return vault.NewVault(fvault.NewCachedVault(store, 100), &disabled.Provider{}, noop.NewTracerProvider())
	}
	lagging, other := newReplicaVault(), newReplicaVault()

	// The lagging replica endorses, which leaves the transaction Busy in the shared store.
	rws, err := lagging.NewRWSet(ctx, "tx1")
	require.NoError(t, err)
	rws.Done()
	c := &Committer{
		logger: logger,
		ChannelConfig: &fake.ChannelConfig{
			FinalityRetries:        3,
			FinalityUnknownTimeout: time.Millisecond,
			WaitForEventTimeout:    time.Minute,
		},
		Vault:           lagging,
		EnvelopeService: &fake.EnvelopeService{},
		listeners:       map[string][]chan FinalityEvent{},
		pollingTimeout:  10 * time.Millisecond,
		tracer:          noop.NewTracerProvider().Tracer("test"),
	}
	vc, _, err := c.Status(ctx, "tx1")
	require.NoError(t, err)
	require.Equal(t, fdriver.Busy, vc)

	// The other replica's delivery commits the block. Nothing reaches the lagging
	// replica's committer, so only a reread of the shared status settles IsFinal.
	rws, err = other.NewRWSet(ctx, "tx1")
	require.NoError(t, err)
	rws.Done()
	require.NoError(t, other.CommitTX(ctx, "tx1", 7, 0))

	// Well above the polling interval, so a replica that rereads settles long before it.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	require.NoError(t, c.IsFinal(ctx, "tx1"))
}

func TestLateReplicaCommitKeepsLatestKey(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	store, err := fvault.OpenSqliteVault("node", t.TempDir())
	require.NoError(t, err)
	defer func() { _ = store.Close() }()
	newReplicaVault := func() *vault.Vault {
		return vault.NewVault(fvault.NewCachedVault(store, 100), &disabled.Provider{}, noop.NewTracerProvider())
	}
	a, b := newReplicaVault(), newReplicaVault()

	cA := &Committer{
		logger: logger,
		ChannelConfig: &fake.ChannelConfig{
			FinalityRetries:        3,
			FinalityUnknownTimeout: time.Millisecond,
			WaitForEventTimeout:    time.Minute,
		},
		Vault:              a,
		EnvelopeService:    &fake.EnvelopeService{},
		listeners:          map[string][]chan FinalityEvent{},
		pollingTimeout:     10 * time.Millisecond,
		tracer:             noop.NewTracerProvider().Tracer("test"),
		metrics:            NewMetrics(noop.NewTracerProvider(), &fake.MetricsProvider{}, "testNet", "testChannel"),
		DependencyResolver: NewSerialDependencyResolver(),
		ProcessorManager: &fake.ProcessorManager{
			ProcessByIDFn: func(context.Context, string, cdriver.TxID) error { return nil },
		},
	}

	cB := &Committer{
		logger: logger,
		ChannelConfig: &fake.ChannelConfig{
			FinalityRetries:        3,
			FinalityUnknownTimeout: time.Millisecond,
			WaitForEventTimeout:    time.Minute,
		},
		Vault:              b,
		EnvelopeService:    &fake.EnvelopeService{},
		listeners:          map[string][]chan FinalityEvent{},
		pollingTimeout:     10 * time.Millisecond,
		tracer:             noop.NewTracerProvider().Tracer("test"),
		metrics:            NewMetrics(noop.NewTracerProvider(), &fake.MetricsProvider{}, "testNet", "testChannel"),
		DependencyResolver: NewSerialDependencyResolver(),
		ProcessorManager: &fake.ProcessorManager{
			ProcessByIDFn: func(context.Context, string, cdriver.TxID) error { return nil },
		},
	}

	rws, err := a.NewRWSet(ctx, "X")
	require.NoError(t, err)
	err = rws.SetState("ns", "K", []byte("vX"))
	require.NoError(t, err)
	rws.Done()

	vc, _, err := cA.Status(ctx, "X")
	require.NoError(t, err)
	require.Equal(t, fdriver.Busy, vc)

	rws, err = b.NewRWSet(ctx, "X")
	require.NoError(t, err)
	err = rws.SetState("ns", "K", []byte("vX"))
	require.NoError(t, err)
	rws.Done()

	isValid, err := cB.CommitEndorserTransaction(ctx, "X", 10, 0, nil, &FinalityEvent{TxID: "X"})
	require.NoError(t, err)
	require.False(t, isValid)

	rws, err = b.NewRWSet(ctx, "Y")
	require.NoError(t, err)
	err = rws.SetState("ns", "K", []byte("vY"))
	require.NoError(t, err)
	rws.Done()

	isValid, err = cB.CommitEndorserTransaction(ctx, "Y", 11, 0, nil, &FinalityEvent{TxID: "Y"})
	require.NoError(t, err)
	require.False(t, isValid)

	isValid, err = cA.CommitEndorserTransaction(ctx, "X", 10, 0, nil, &FinalityEvent{TxID: "X"})
	require.NoError(t, err)
	require.True(t, isValid)

	qe, _ := a.NewQueryExecutor(ctx)
	defer func() { require.NoError(t, qe.Done()) }()
	vr, err := qe.GetState(ctx, "ns", "K")
	require.NoError(t, err)

	require.Equal(t, []byte("vY"), vr.Raw)
}

// A replica that discarded a transaction reports it Valid once another replica commits it:
// a block that marks a discarded transaction valid overrides the discard, so an Invalid
// status in one replica's cache may be stale.
func TestStatusSeesCommitOfTxAnotherReplicaDiscarded(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	store, err := fvault.OpenSqliteVault("node", t.TempDir())
	require.NoError(t, err)
	defer func() { _ = store.Close() }()
	newReplicaVault := func() *vault.Vault {
		return vault.NewVault(fvault.NewCachedVault(store, 100), &disabled.Provider{}, noop.NewTracerProvider())
	}
	discarding, other := newReplicaVault(), newReplicaVault()
	c := &Committer{
		logger:          logger,
		ChannelConfig:   &fake.ChannelConfig{},
		Vault:           discarding,
		EnvelopeService: &fake.EnvelopeService{},
	}

	rws, err := discarding.NewRWSet(ctx, "tx1")
	require.NoError(t, err)
	rws.Done()
	require.NoError(t, c.DiscardTx(ctx, "tx1", "discarded locally"))
	vc, _, err := c.Status(ctx, "tx1")
	require.NoError(t, err)
	require.Equal(t, fdriver.Invalid, vc)

	// The block marks the transaction valid, and the other replica commits it first.
	rws, err = other.NewRWSet(ctx, "tx1")
	require.NoError(t, err)
	rws.Done()
	require.NoError(t, other.CommitTX(ctx, "tx1", 7, 0))

	vc, _, err = c.Status(ctx, "tx1")
	require.NoError(t, err)
	require.Equal(t, fdriver.Valid, vc)
}
