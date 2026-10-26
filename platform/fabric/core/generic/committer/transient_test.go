/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	commoncommitter "github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/committer"
	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer/fake"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	vault2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/storage/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

// flakyStore fails the first Store call with a lost connection, as a database
// restart in the middle of a commit would, and passes every later call through.
type flakyStore struct {
	cdriver.VaultStore
	failed atomic.Bool
}

func (s *flakyStore) Store(ctx context.Context, txIDs []cdriver.TxID, writes cdriver.Writes, metaWrites cdriver.MetaWrites) error {
	if s.failed.CompareAndSwap(false, true) {
		return errors.Wrap(dbdriver.ErrConnectionLost, "connection reset by peer")
	}
	return s.VaultStore.Store(ctx, txIDs, writes, metaWrites)
}

// TestCommitConfigRetriesATransientVaultFailure runs CommitConfig against a real
// vault, because what decides the retry is the status the failed attempt leaves
// in the store, and a stub would report whatever the test assumed.
func TestCommitConfigRetriesATransientVaultFailure(t *testing.T) {
	t.Parallel()

	store, err := vault2.OpenMemoryVault(t.Name())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	v := vault.NewVault(vault2.NewCachedVault(&flakyStore{VaultStore: store}, 0), &disabled.Provider{}, noop.NewTracerProvider())

	c := &Committer{
		logger:            logger,
		Vault:             v,
		ChannelConfig:     &fake.ChannelConfig{IDValue: "ch"},
		MembershipService: &fake.MembershipService{},
		ProcessorManager: &fake.ProcessorManager{
			ProcessByIDFn: func(context.Context, string, cdriver.TxID) error { return nil },
		},
	}
	env := configEnvelope(1)
	txID := ConfigTXPrefix + "1"

	err = c.CommitConfig(t.Context(), 1, []byte("raw"), env)
	require.ErrorIs(t, err, dbdriver.ErrConnectionLost)
	require.Equal(t, classRetry, classify(err), "a lost connection must stay retryable")

	vc, _, err := v.Status(t.Context(), txID)
	require.NoError(t, err)
	require.Equal(t, fdriver.Busy, vc, "a transient failure must leave the config tx for the retry, not discard it")

	require.NoError(t, c.CommitConfig(t.Context(), 1, []byte("raw"), env),
		"committing the block again must apply the config")
	vc, _, err = v.Status(t.Context(), txID)
	require.NoError(t, err)
	require.Equal(t, fdriver.Valid, vc)
}

// TestCommitUnknownReturnsEnvelopeLookupError checks that a failed envelope
// lookup reaches the caller as a retryable error. Read as "no envelope", the
// committer would fetch the transaction from the ledger and, when no filter
// selects it, skip it without error.
func TestCommitUnknownReturnsEnvelopeLookupError(t *testing.T) {
	t.Parallel()

	store := &mock.EnvelopeStore{}
	store.ExistsEnvelopeReturns(false, errors.Wrap(dbdriver.ErrConnectionLost, "connection refused"))
	var fetched atomic.Bool
	c := &Committer{
		logger:          logger,
		Vault:           &fake.Vault{},
		EnvelopeService: transaction.NewEnvelopeService(store, "net", "ch"),
		Ledger: &fake.Ledger{
			GetByIDFn: func(string) (fdriver.ProcessedTransaction, error) {
				fetched.Store(true)
				return nil, errors.New("unexpected ledger fetch")
			},
		},
	}

	err := c.CommitTX(t.Context(), "tx1", 1, 0, (*common.Envelope)(nil))
	require.ErrorIs(t, err, dbdriver.ErrConnectionLost)
	require.Equal(t, classRetry, classify(err))
	require.False(t, fetched.Load(), "a failed lookup must not be treated as a missing envelope")
}

// TestCommitUnknownReturnsTransientFilterError checks that a transient failure
// while deciding whether to track an unknown transaction is retried rather than
// read as "not tracked", which would skip the transaction for good.
func TestCommitUnknownReturnsTransientFilterError(t *testing.T) {
	t.Parallel()

	c := &Committer{
		logger: logger,
		Vault: &fake.Vault{
			StatusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
				return fdriver.Unknown, "", errors.Wrap(dbdriver.ErrConnectionLost, "connection refused")
			},
		},
		EnvelopeService: &fake.EnvelopeService{},
		RWSetLoaderService: &fake.RWSetLoader{
			InspectFromEnvFn: func(context.Context, cdriver.TxID, []byte) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
				return &fake.RWSet{}, nil, nil
			},
		},
		TransactionFilters: commoncommitter.NewAggregatedTransactionFilter(),
	}

	err := c.commitUnknown(t.Context(), "tx1", 1, 0, &common.Envelope{})
	require.ErrorIs(t, err, dbdriver.ErrConnectionLost)
	require.Equal(t, classRetry, classify(err))
}
