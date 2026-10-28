/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	nwocontext "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/context"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/network"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
)

// newDeleteVaultPlatform returns a platform with one FSC node named alice, configured by opts,
// and a file for each of its SQLite stores and their WAL siblings.
func newDeleteVaultPlatform(t *testing.T, opts ...node.Option) (*Platform, string) {
	t.Helper()

	fscNode := node.NewNode("alice")
	require.NoError(t, fscNode.Options.Parse(opts...))
	p := &Platform{Network: &network.Network{
		Context: nwocontext.New(t.TempDir(), 0, nil),
		Peers:   []*topology.Peer{{Name: "alice", Type: topology.FSCPeer, FSCNode: fscNode}},
	}}
	dir := p.Network.NodeStorages(fscNode.ReplicaUniqueNames()[0])
	require.NoError(t, os.MkdirAll(dir, 0o755))
	for _, f := range []string{"default.sqlite", "fabric_vault.sqlite", "fabric_vault.sqlite-wal", "fabric_vault.sqlite-shm"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, f), nil, 0o644))
	}
	return p, dir
}

func TestDeleteVault(t *testing.T) {
	t.Parallel()

	p, dir := newDeleteVaultPlatform(t, WithDefaultSqlitePersistence())
	p.DeleteVault("alice")

	files, err := filepath.Glob(filepath.Join(dir, "*"))
	require.NoError(t, err)
	require.Equal(t, []string{filepath.Join(dir, "default.sqlite")}, files, "only the vault is removed")
}

func TestDeleteVault_SharedPersistence(t *testing.T) {
	t.Parallel()

	for name, opt := range map[string]node.Option{
		"postgres default": WithDefaultPostgresPersistence(postgres.DefaultConfig()),
		"named vault":      WithStorePersistence(network.VaultPersistenceKey, "default"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			p, dir := newDeleteVaultPlatform(t, opt)
			p.DeleteVault("alice")

			files, err := filepath.Glob(filepath.Join(dir, "*"))
			require.NoError(t, err)
			require.Len(t, files, 4, "nothing is removed")
		})
	}
}

func TestPostgresOptions_UndefinedPersistence(t *testing.T) {
	t.Parallel()

	o := &node.Options{}
	require.ErrorContains(t, o.Parse(WithTablePrefix("second", "p")), "postgres persistence [second] is not defined")
	require.ErrorContains(t, o.Parse(WithPostgresTLS("second")), "postgres persistence [second] is not defined")
}

func TestPostgresOptions(t *testing.T) {
	t.Parallel()

	o := &node.Options{}
	cfg := postgres.DefaultConfig()
	require.NoError(t, o.Parse(
		WithPostgresPersistence("second", cfg),
		WithTablePrefix("second", "borrower"),
		WithPostgresTLS("second"),
		WithStorePersistence("fsc_kvs", "second"),
	))

	require.Equal(t, &node.SQLOpts{DataSource: cfg.DataSource(), TablePrefix: "borrower", TLS: true}, o.GetPostgresPersistence("second"))
	name, ok := o.GetPersistenceName("fsc_kvs")
	require.True(t, ok)
	require.EqualValues(t, "second", name)
}
