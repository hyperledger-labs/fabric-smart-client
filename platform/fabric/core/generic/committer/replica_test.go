/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"context"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer/fake"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	vault2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/storage/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

// sequenceMembership is a membership service that remembers the sequence of the
// configuration it holds, as the real one does, without parsing a channel group.
type sequenceMembership struct {
	fake.MembershipService
	loaded bool
	seq    uint64
	// onUpdate, if set, runs after a configuration is installed.
	onUpdate func()
}

func (m *sequenceMembership) Update(env *common.Envelope) error {
	seq, err := configSequenceOf(env)
	if err != nil {
		return err
	}
	m.loaded, m.seq = true, seq
	if m.onUpdate != nil {
		m.onUpdate()
	}
	return nil
}

func (m *sequenceMembership) ConfigSequence() (uint64, error) {
	if !m.loaded {
		return 0, fdriver.ErrNotInitialized
	}
	return m.seq, nil
}

// replica builds a committer over store with its own membership service, as
// each replica of a node sharing one vault has.
func replica(store cdriver.VaultStore) (*Committer, *sequenceMembership) {
	ms := &sequenceMembership{}
	return &Committer{
		logger:            logger,
		tracer:            noop.NewTracerProvider().Tracer("test"),
		Vault:             vault.NewVault(vault2.NewCachedVault(store, 0), &disabled.Provider{}, noop.NewTracerProvider()),
		ChannelConfig:     &fake.ChannelConfig{IDValue: "ch"},
		MembershipService: ms,
		ProcessorManager: &fake.ProcessorManager{
			ProcessByIDFn: func(context.Context, string, cdriver.TxID) error { return nil },
		},
	}, ms
}

// rawConfig is the block-level bytes CommitConfig stores, which
// ReloadConfigTransactions reads back as an envelope.
func rawConfig(seq uint64) []byte {
	return protoutil.MarshalOrPanic(configEnvelope(seq))
}

func requireSequence(t *testing.T, ms *sequenceMembership, want uint64) {
	t.Helper()
	got, err := ms.ConfigSequence()
	require.NoError(t, err)
	require.Equal(t, want, got)
}

// TestCommitConfigAcrossReplicasSharingAVault checks that every replica of a node
// applies each configuration block to its own membership service, although the
// vault they share records it as committed after the first replica's commit.
func TestCommitConfigAcrossReplicasSharingAVault(t *testing.T) {
	t.Parallel()

	newStore := func(t *testing.T, name string) cdriver.VaultStore {
		t.Helper()
		store, err := vault2.OpenMemoryVault(name)
		require.NoError(t, err)
		t.Cleanup(func() { _ = store.Close() })
		return store
	}

	t.Run("a replica that receives a block after another committed it", func(t *testing.T) {
		t.Parallel()
		store := newStore(t, "replicas_live")
		a, msA := replica(store)
		b, msB := replica(store)

		for seq := range uint64(2) {
			require.NoError(t, a.CommitConfig(t.Context(), seq, rawConfig(seq), configEnvelope(seq)))
			require.NoError(t, b.CommitConfig(t.Context(), seq, rawConfig(seq), configEnvelope(seq)))
			requireSequence(t, msA, seq)
			requireSequence(t, msB, seq)
		}

		require.NoError(t, b.CommitConfig(t.Context(), 0, rawConfig(0), configEnvelope(0)))
		requireSequence(t, msB, 1)
	})

	t.Run("a replica that reloads while another is committing", func(t *testing.T) {
		t.Parallel()
		store := newStore(t, "replicas_restart")
		a, _ := replica(store)
		require.NoError(t, a.CommitConfig(t.Context(), 0, rawConfig(0), configEnvelope(0)))

		// A's commit of configuration 1 fails part-way and leaves it Busy.
		aFlaky, _ := replica(&flakyStore{VaultStore: store})
		require.ErrorIs(t, aFlaky.CommitConfig(t.Context(), 1, rawConfig(1), configEnvelope(1)), dbdriver.ErrConnectionLost)

		// B restarts and loads configuration 0 only.
		b, msB := replica(store)
		require.NoError(t, b.ReloadConfigTransactions())
		requireSequence(t, msB, 0)

		// A's retry marks configuration 1 Valid, then B's delivery replays it.
		require.NoError(t, aFlaky.CommitConfig(t.Context(), 1, rawConfig(1), configEnvelope(1)))
		require.NoError(t, b.CommitConfig(t.Context(), 1, rawConfig(1), configEnvelope(1)))
		requireSequence(t, msB, 1)
	})

	t.Run("replicas committing the same block at once", func(t *testing.T) {
		t.Parallel()
		store := newStore(t, "replicas_concurrent")
		a, _ := replica(store)
		b, msB := replica(store)

		// A commits the block while B is applying it, so B finds it Valid when
		// it comes to record it. The package-level commitConfigMutex would
		// serialize the two calls in one process, so A records directly.
		msB.onUpdate = func() {
			require.NoError(t, a.applyConfigCommit(t.Context(), ConfigTXPrefix+"0", 0, 0, rawConfig(0)))
		}
		require.NoError(t, b.CommitConfig(t.Context(), 0, rawConfig(0), configEnvelope(0)))
		requireSequence(t, msB, 0)
	})
}
