/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
)

// countingSDK counts Install calls and keeps the context Start got.
type countingSDK struct {
	installs int
	ctx      context.Context
}

func (s *countingSDK) Install() error                  { s.installs++; return nil }
func (s *countingSDK) Start(ctx context.Context) error { s.ctx = ctx; return nil }

// Start does not install again after Install.
func TestNode_InstallThenStart(t *testing.T) {
	t.Parallel()
	n := newTestNode()
	sdk := &countingSDK{}
	n.AddSDK(sdk)
	require.NoError(t, n.Install())
	require.NoError(t, n.Install())
	require.NoError(t, n.Start(t.Context()))
	require.Equal(t, 1, sdk.installs)
	n.Stop()
}

// Cancelling the context passed to Start does not end the node: only Stop does, after
// the drain.
func TestNode_StartContextCancelKeepsNode(t *testing.T) {
	t.Parallel()
	n := newTestNode()
	sdk := &countingSDK{}
	n.AddSDK(sdk)
	ctx, cancel := context.WithCancel(t.Context())
	require.NoError(t, n.Start(ctx))
	cancel()
	require.NoError(t, sdk.ctx.Err())
	n.Stop()
	require.ErrorIs(t, sdk.ctx.Err(), context.Canceled)
}

// Migrate refuses a node whose SDKs are installed and runs no step.
func TestNode_MigrateAfterInstall(t *testing.T) {
	t.Parallel()
	n := newTestNode()
	sdk := &countingSDK{}
	n.AddSDK(sdk)
	require.NoError(t, n.Install())
	require.Error(t, n.Migrate(t.Context(), false))
	require.Equal(t, 1, sdk.installs)
}

// contractStore is a store whose step 2 is a contract step.
type contractStore struct{ steps int }

func (contractStore) StoreName() string { return "cli_s" }
func (contractStore) CreateSchema() error {
	return errors.New("CreateSchema must not be called on a Versioned store")
}

func (s contractStore) Steps() []migrate.Step {
	return []migrate.Step{
		{Version: 1, Name: "create", SQL: []string{"CREATE TABLE cli_s (id INT)"}},
		{Version: 2, Name: "contract", Contract: true, SQL: []string{"CREATE TABLE cli_s2 (id INT)"}},
	}[:s.steps]
}

// storeSDK opens one store on Install, as the SDKs do inside Migrate: with no
// options of its own, so only the options passed to Migrate reach it.
type storeSDK struct {
	mockSDK
	db *sql.DB
}

func (s *storeSDK) Install() error {
	return migrate.Migrate(context.Background(), s.db, migrate.SQLite, "cli", contractStore{steps: 2})
}

// The options passed to Migrate reach the stores the SDKs open, and only for
// that call. Not parallel: Migrate sets them process-wide.
func TestNode_MigrateAppliesOptions(t *testing.T) { //nolint:paralleltest // process-wide migrate options
	dsn := "file:" + filepath.Join(t.TempDir(), "m.db")
	open := func() *sql.DB {
		db, err := sql.Open("sqlite", dsn)
		require.NoError(t, err)
		db.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = db.Close() })
		return db
	}
	// A binary with step 1 only has opened the store as node "old", from another process.
	require.NoError(t, migrate.Migrate(t.Context(), open(), migrate.SQLite, "cli", contractStore{steps: 1}, migrate.WithNode("old")))
	db := open()
	run := func(opts ...migrate.Option) error {
		n, err := NewFromConfPathE(filepath.Join("..", "platform", "view", "services", "config", "testdata"))
		require.NoError(t, err)
		n.AddSDK(&storeSDK{db: db})
		require.NoError(t, n.RegisterService(storeOpenerFunc(func(context.Context) error { return nil })))
		return n.Migrate(t.Context(), false, opts...)
	}

	require.NoError(t, run(), "without WithContract the contract step waits")
	require.ErrorIs(t, run(migrate.WithContract()), migrate.ErrOlderOpener)
	require.NoError(t, run(migrate.WithContract(), migrate.WithForgetOpeners("old")))
	var readableBy int
	require.NoError(t, db.QueryRow("SELECT readable_by FROM cli_schema_versions").Scan(&readableBy))
	require.Equal(t, 2, readableBy)
}

// A node that ran Migrate refuses Install, Start and a second Migrate instead of installing
// its SDKs a second time.
func TestNode_MigratedNodeIsSingleUse(t *testing.T) {
	t.Parallel()
	n, err := NewFromConfPathE(filepath.Join("..", "platform", "view", "services", "config", "testdata"))
	require.NoError(t, err)
	sdk := &countingSDK{}
	n.AddSDK(sdk)
	require.NoError(t, n.RegisterService(storeOpenerFunc(func(context.Context) error { return nil })))
	require.NoError(t, n.Migrate(t.Context(), false))
	require.ErrorContains(t, n.Start(t.Context()), "migrated")
	require.ErrorContains(t, n.Migrate(t.Context(), false), "migrated")
	require.Equal(t, 1, sdk.installs)
}

// A panic in an SDK's Start fails Start without stopping the node; the caller stops it, as
// after any other error.
func TestNode_StartPanicDoesNotStop(t *testing.T) {
	t.Parallel()
	n := newTestNode()
	d := &countingDrainer{}
	require.NoError(t, n.RegisterService(d))
	n.AddSDK(panicStartSDK{})
	require.ErrorContains(t, n.Start(t.Context()), "start triggered panic")
	require.Zero(t, d.drains.Load())
	n.Stop()
	require.Equal(t, int32(1), d.drains.Load())
}

type panicStartSDK struct{}

func (panicStartSDK) Install() error              { return nil }
func (panicStartSDK) Start(context.Context) error { panic("boom") }

type countingDrainer struct{ drains atomic.Int32 }

func (d *countingDrainer) Drain(context.Context) error { d.drains.Add(1); return nil }
