/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer/fake"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	fvault "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/storage/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
)

// A node that endorsed a transaction and restarted before it was ordered keeps
// it Busy. Discarding it then is final: when Fabric later orders it as valid,
// the vault keeps it Invalid and skips it, while the finality event still
// reports Fabric's verdict.
func TestDiscardedBusyTxSkippedWhenOrderedLater(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	dir := t.TempDir()

	store, err := fvault.OpenSqliteVault("node", dir)
	require.NoError(t, err)
	v := vault.NewVault(fvault.NewCachedVault(store, 0), &disabled.Provider{}, noop.NewTracerProvider())
	rws, err := v.NewRWSet(ctx, "tx1")
	require.NoError(t, err)
	rws.Done()
	require.NoError(t, store.Close())

	// Restart: the read-write set held in memory is gone, the Busy status is not.
	store, err = fvault.OpenSqliteVault("node", dir)
	require.NoError(t, err)
	defer func() { _ = store.Close() }()
	c := &Committer{
		logger:          logger,
		ChannelConfig:   &fake.ChannelConfig{IDValue: "ch"},
		Vault:           vault.NewVault(fvault.NewCachedVault(store, 0), &disabled.Provider{}, noop.NewTracerProvider()),
		EnvelopeService: &fake.EnvelopeService{},
	}
	require.False(t, c.Vault.RWSExists(ctx, "tx1"))
	vc, _, err := c.Status(ctx, "tx1")
	require.NoError(t, err)
	require.Equal(t, fdriver.Busy, vc)

	require.NoError(t, c.DiscardTx(ctx, "tx1", "stale"))

	metadata := make([][]byte, int(common.BlockMetadataIndex_TRANSACTIONS_FILTER)+1)
	metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = []byte{uint8(peer.TxValidationCode_VALID)}
	event, err := c.HandleEndorserTransaction(ctx, &common.BlockMetadata{Metadata: metadata}, CommitTx{
		TxID: "tx1", BlkNum: 7, Envelope: &common.Envelope{},
	})
	require.NoError(t, err)
	require.Equal(t, fdriver.Valid, event.ValidationCode)
	require.NoError(t, event.Err)

	vc, _, err = c.Status(ctx, "tx1")
	require.NoError(t, err)
	require.Equal(t, fdriver.Invalid, vc)
}
