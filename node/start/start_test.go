/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// package start (internal) to access unexported serve and startCmd functions.
package start

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
)

type mockStartNode struct {
	startErr error
	ch       chan error
	// stops receives a value on each Stop call; stop, if set, runs in Stop.
	stops chan struct{}
	stop  func()
}

func (*mockStartNode) ID() string     { return "test-id" }
func (m *mockStartNode) Start() error { return m.startErr }
func (m *mockStartNode) Stop() {
	m.stops <- struct{}{}
	if m.stop != nil {
		m.stop()
	}
}
func (m *mockStartNode) Callback() chan<- error { return m.ch }

func newMockStartNode(startErr error) *mockStartNode {
	return &mockStartNode{ch: make(chan error, 1), stops: make(chan struct{}, 2), startErr: startErr}
}

func TestCmd(t *testing.T) {
	t.Parallel()
	n := newMockStartNode(nil)
	cmd := Cmd(n)
	require.Equal(t, nodeFuncName, cmd.Use)
	found := false
	for _, sub := range cmd.Commands() {
		if sub.Use == "start" {
			found = true
			break
		}
	}
	require.True(t, found, "start subcommand should be registered")
}

func TestStartCmd_TrailingArgs(t *testing.T) {
	t.Parallel()
	cmd := startCmd(newMockStartNode(nil))
	err := cmd.RunE(cmd, []string{"unexpected"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "trailing args detected")
}

func TestStartCmd_NoArgs(t *testing.T) {
	t.Parallel()
	n := newMockStartNode(errors.New("stop early"))
	cmd := startCmd(n)
	err := cmd.RunE(cmd, []string{})
	require.Error(t, err)
	<-n.ch
}

func TestServe_StartError(t *testing.T) {
	t.Parallel()
	n := newMockStartNode(errors.New("start failed"))
	err := serve(n)
	require.Error(t, err)
	require.Contains(t, err.Error(), "start failed")
	require.Error(t, <-n.ch)
}

func TestServe_InvalidProfilerEnv(t *testing.T) { //nolint:paralleltest
	t.Setenv("FSCNODE_PROFILER", "not-a-bool")
	n := newMockStartNode(errors.New("stop early"))
	err := serve(n)
	require.Error(t, err)
	<-n.ch
}

func TestServe_ProfilerEnabled(t *testing.T) { //nolint:paralleltest
	t.Setenv("FSCNODE_PROFILER", "true")
	t.Setenv("FSCNODE_CFG_PATH", t.TempDir())
	n := newMockStartNode(errors.New("stop after profiler"))
	err := serve(n)
	require.Error(t, err)
	<-n.ch
}

// A config path that is a regular file makes the profiler's MkdirAll fail, so
// serve() bails out on profiler.Start() before ever reaching n.Start().
func TestServe_ProfilerStartError(t *testing.T) { //nolint:paralleltest
	file := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(file, nil, 0o644))
	t.Setenv("FSCNODE_PROFILER", "true")
	t.Setenv("FSCNODE_CFG_PATH", file)

	n := newMockStartNode(nil)
	require.ErrorContains(t, serve(n), "failed to create profile directory")
	require.Error(t, <-n.ch)
}

func TestServe_HappyPath(t *testing.T) { //nolint:paralleltest // signals the process
	n := newMockStartNode(nil)
	// Once serve() sends nil on the callback channel we know Start() succeeded
	// and it is waiting for a signal. Send SIGTERM so serve() stops the node and
	// returns.
	go func() {
		select {
		case <-n.ch:
			_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
		case <-t.Context().Done():
		}
	}()
	require.NoError(t, serve(n))
}

// A second signal while the first Stop still drains calls Stop again, which skips the rest
// of the drain.
func TestServe_SecondSignalSkipsDrain(t *testing.T) { //nolint:paralleltest // signals the process
	n := newMockStartNode(nil)
	release := make(chan struct{})
	var calls atomic.Int32
	n.stop = func() {
		if calls.Add(1) == 1 {
			<-release // the first Stop drains until the second one arrives
			return
		}
		close(release)
	}
	go func() {
		<-n.ch
		_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
		<-n.stops
		_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
	}()
	require.NoError(t, serve(n))
	<-n.stops
}

type mockMigrateNode struct {
	*mockStartNode
	checkOnly []bool
	err       error
}

func (m *mockMigrateNode) Migrate(_ context.Context, checkOnly bool) error {
	m.checkOnly = append(m.checkOnly, checkOnly)
	return m.err
}

// Cmd adds migrate for a node that supports it, and --check selects checkOnly.
func TestMigrateCmd(t *testing.T) {
	t.Parallel()

	require.Len(t, Cmd(newMockStartNode(nil)).Commands(), 1, "a node without Migrate gets no migrate command")

	n := &mockMigrateNode{mockStartNode: newMockStartNode(nil)}
	cmd := Cmd(n)
	cmd.SetArgs([]string{"migrate"})
	require.NoError(t, cmd.Execute())
	cmd.SetArgs([]string{"migrate", "--check"})
	require.NoError(t, cmd.Execute())
	require.Equal(t, []bool{false, true}, n.checkOnly)
}

// Each outcome of Migrate gives its exit code; a joined error, one per store,
// takes the most severe code.
func TestMigrateCmdExitCodes(t *testing.T) {
	t.Parallel()

	pending := errors.Join(fmt.Errorf("store [a]: %w", migrate.ErrPending), fmt.Errorf("store [b]: %w", migrate.ErrPending))
	for name, tc := range map[string]struct {
		err  error
		code int
	}{
		"pending":              {pending, ExitPending},
		"too new":              {fmt.Errorf("store [a]: %w", migrate.ErrSchemaTooNew), ExitSchemaTooNew},
		"older opener":         {fmt.Errorf("store [a]: %w", migrate.ErrOlderOpener), ExitOlderOpener},
		"older and too new":    {errors.Join(migrate.ErrOlderOpener, migrate.ErrSchemaTooNew), ExitSchemaTooNew},
		"older and pending":    {errors.Join(pending, migrate.ErrOlderOpener), ExitOlderOpener},
		"unreachable":          {errors.New("database [h:5432/db] not reachable"), ExitFailed},
		"pending and too new":  {errors.Join(pending, migrate.ErrSchemaTooNew), ExitSchemaTooNew},
		"pending and failed":   {errors.Join(migrate.ErrPending, errors.New("lock")), ExitFailed},
		"wrapped joined error": {fmt.Errorf("open: %w", pending), ExitPending},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			n := &mockMigrateNode{mockStartNode: newMockStartNode(nil), err: tc.err}
			cmd := Cmd(n)
			cmd.SetArgs([]string{"migrate", "--check"})
			var e *ExitError
			require.ErrorAs(t, cmd.Execute(), &e)
			require.Equal(t, tc.code, e.Code)
			require.ErrorIs(t, e, tc.err)
		})
	}
}

// Invalid arguments exit 2 without calling the migrator; --help never calls it.
func TestMigrateCmdUsage(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{{"migrate", "extra"}, {"migrate", "--force"}, {"migrate", "--check=maybe"}, {"migrate", "--check", "--forget-opener", "n"}} {
		n := &mockMigrateNode{mockStartNode: newMockStartNode(nil)}
		cmd := Cmd(n)
		cmd.SetArgs(args)
		cmd.SetErr(io.Discard)
		var e *ExitError
		require.ErrorAs(t, cmd.Execute(), &e, "%v", args)
		require.Equal(t, ExitUsage, e.Code, "%v", args)
		require.Empty(t, n.checkOnly, "%v called the migrator", args)
	}

	n := &mockMigrateNode{mockStartNode: newMockStartNode(nil)}
	cmd := Cmd(n)
	cmd.SetArgs([]string{"migrate", "--help"})
	cmd.SetOut(io.Discard)
	require.NoError(t, cmd.Execute())
	require.Empty(t, n.checkOnly)
}

// contractStore is a store whose step 2 is a contract step.
type contractStore struct{}

func (contractStore) StoreName() string { return "cli_s" }
func (contractStore) CreateSchema() error {
	return errors.New("CreateSchema must not be called on a Versioned store")
}

func (contractStore) Steps() []migrate.Step {
	return []migrate.Step{
		{Version: 1, Name: "create", SQL: []string{"CREATE TABLE cli_s (id INT)"}},
		{Version: 2, Name: "contract", Contract: true, SQL: []string{"CREATE TABLE cli_s2 (id INT)"}},
	}
}

// storeMigrateNode opens one store, as the SDKs do inside node migrate: with
// no options of its own, so only the command's flags reach migrate.
type storeMigrateNode struct {
	*mockStartNode
	db *sql.DB
}

func (m *storeMigrateNode) Migrate(ctx context.Context, checkOnly bool) error {
	var opts []migrate.Option
	if checkOnly {
		opts = append(opts, migrate.WithCheckOnly())
	}
	return migrate.Migrate(ctx, m.db, migrate.SQLite, "cli", contractStore{}, opts...)
}

// --contract and --forget-opener reach the stores the node opens, and only
// for the duration of the command. Not parallel: the flags are set
// process-wide while the command runs.
func TestMigrateCmdContract(t *testing.T) { //nolint:paralleltest // sets process-wide migrate options
	dsn := "file:" + filepath.Join(t.TempDir(), "m.db")
	open := func() *sql.DB {
		db, err := sql.Open("sqlite", dsn)
		require.NoError(t, err)
		db.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = db.Close() })
		return db
	}
	// A binary with step 1 only has opened the store as node "old". It is
	// another process, so it uses another pool.
	require.NoError(t, migrate.Migrate(t.Context(), open(), migrate.SQLite, "cli", &stepsStore{contractStore{}, 1}, migrate.WithNode("old")))
	db := open()

	run := func(args ...string) error {
		cmd := Cmd(&storeMigrateNode{mockStartNode: newMockStartNode(nil), db: db})
		cmd.SetArgs(append([]string{"migrate"}, args...))
		return cmd.Execute()
	}
	require.NoError(t, run(), "without --contract the contract step waits")
	require.NoError(t, run("--check"), "a held-back contract step is not pending")
	var e *ExitError
	require.ErrorAs(t, run("--check", "--contract"), &e)
	require.Equal(t, ExitPending, e.Code)
	require.ErrorAs(t, run("--contract"), &e)
	require.Equal(t, ExitOlderOpener, e.Code)
	require.ErrorContains(t, e, "node [old] knows step 1")
	require.NoError(t, run("--contract", "--forget-opener", "old"))
	require.NoError(t, run("--check", "--contract"))
	var readableBy int
	require.NoError(t, db.QueryRow("SELECT readable_by FROM cli_schema_versions").Scan(&readableBy))
	require.Equal(t, 2, readableBy)
}

// stepsStore is a store with only the first n steps of another.
type stepsStore struct {
	contractStore
	n int
}

func (s *stepsStore) Steps() []migrate.Step { return s.contractStore.Steps()[:s.n] }
