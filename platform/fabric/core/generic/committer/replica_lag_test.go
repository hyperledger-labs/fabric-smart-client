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

	cvault "github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault"
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

// A replica that commits a block late skips a transaction another replica sharing
// the vault store already committed, so it does not set a key back to that
// transaction's value and version. This covers the committer; the vault store
// skips the writes of an already Valid transaction too (TTestLateReplicaCommit).
func TestLateReplicaCommitKeepsLatestKey(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	store, err := fvault.OpenSqliteVault("node", t.TempDir())
	require.NoError(t, err)
	defer func() { _ = store.Close() }()
	newReplica := func() (*vault.Vault, *Committer) {
		v := vault.NewVault(fvault.NewCachedVault(store, 100), &disabled.Provider{}, noop.NewTracerProvider())
		return v, &Committer{
			logger:        logger,
			ChannelConfig: &fake.ChannelConfig{},
			Vault:         v,
			ProcessorManager: &fake.ProcessorManager{
				ProcessByIDFn: func(context.Context, string, cdriver.TxID) error { return nil },
			},
		}
	}
	a, cA := newReplica()
	b, cB := newReplica()
	endorse := func(v *vault.Vault, txID, value string) {
		rws, err := v.NewRWSet(ctx, txID)
		require.NoError(t, err)
		require.NoError(t, rws.SetState("ns", "K", []byte(value)))
		rws.Done()
	}

	// Replica A endorses X and sees it Busy.
	endorse(a, "X", "vX")
	vc, _, err := cA.Status(ctx, "X")
	require.NoError(t, err)
	require.Equal(t, fdriver.Busy, vc)

	// Replica B commits X in block 10 and Y in block 11, both writing K. B holds
	// X's read-write set too, so it commits X without an envelope.
	endorse(b, "X", "vX")
	alreadyValid, err := cB.CommitEndorserTransaction(ctx, "X", 10, 0, nil, &FinalityEvent{TxID: "X"})
	require.NoError(t, err)
	require.False(t, alreadyValid)
	endorse(b, "Y", "vY")
	alreadyValid, err = cB.CommitEndorserTransaction(ctx, "Y", 11, 0, nil, &FinalityEvent{TxID: "Y"})
	require.NoError(t, err)
	require.False(t, alreadyValid)

	// Replica A reaches block 10 and skips X.
	alreadyValid, err = cA.CommitEndorserTransaction(ctx, "X", 10, 0, nil, &FinalityEvent{TxID: "X"})
	require.NoError(t, err)
	require.True(t, alreadyValid)

	qe, err := a.NewQueryExecutor(ctx)
	require.NoError(t, err)
	defer func() { require.NoError(t, qe.Done()) }()
	vr, err := qe.GetState(ctx, "ns", "K")
	require.NoError(t, err)
	require.Equal(t, []byte("vY"), vr.Raw)
	require.Equal(t, cvault.BlockTxIndexToBytes(11, 0), vr.Version)
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
