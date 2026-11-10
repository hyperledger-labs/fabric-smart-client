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
