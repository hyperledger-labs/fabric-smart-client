/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// package cli (internal) to access unexported serve and startCmd functions.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
)

// fakeNode records the lifecycle calls the commands make, in order, on events.
type fakeNode struct {
	installErr, startErr, migrateErr, cfgErr error
	cfg                                      driver.ConfigService
	events                                   chan string
	// stop, if set, runs in Stop.
	stop      func()
	checkOnly []bool
	opts      [][]migrate.Option
}

func newFakeNode() *fakeNode { return &fakeNode{events: make(chan string, 16)} }

func (n *fakeNode) ID() string                            { n.events <- "id"; return "test-id" }
func (n *fakeNode) Install() error                        { n.events <- "install"; return n.installErr }
func (n *fakeNode) Start(context.Context) error           { n.events <- "start"; return n.startErr }
func (n *fakeNode) Config() (driver.ConfigService, error) { return n.cfg, n.cfgErr }
func (n *fakeNode) Stop() {
	n.events <- "stop"
	if n.stop != nil {
		n.stop()
	}
}

func (n *fakeNode) Migrate(_ context.Context, checkOnly bool, opts ...migrate.Option) error {
	n.checkOnly = append(n.checkOnly, checkOnly)
	n.opts = append(n.opts, opts)
	return n.migrateErr
}

// drain returns the events recorded so far.
func (n *fakeNode) drain() []string {
	var got []string
	for {
		select {
		case e := <-n.events:
			got = append(got, e)
		default:
			return got
		}
	}
}

// waitThenSignal sends SIGTERM to the process once the node logged that it started.
func waitThenSignal(t *testing.T, n *fakeNode) {
	t.Helper()
	go func() {
		for e := range n.events {
			if e == "id" {
				_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
				return
			}
		}
	}()
}

func TestNodeCmd(t *testing.T) {
	t.Parallel()
	var names []string
	for _, sub := range nodeCmd(newFakeNode(), nil).Commands() {
		names = append(names, sub.Name())
	}
	require.ElementsMatch(t, []string{"start", "migrate", "peers"}, names)
}

func TestStartCmd_TrailingArgs(t *testing.T) {
	t.Parallel()
	cmd := startCmd(newFakeNode(), nil)
	err := cmd.RunE(cmd, []string{"unexpected"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "trailing args detected")
}

// setup runs after Install and before Start, so views it registers exist before the node
// serves; the ready line comes after Start.
func TestServe_SetupBeforeStart(t *testing.T) { //nolint:paralleltest // signals the process
	n := newFakeNode()
	setup := func() error { n.events <- "setup"; return nil }
	done := make(chan []string)
	go func() {
		var events []string
		for e := range n.events {
			events = append(events, e)
			if e == "id" {
				_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
			}
			if e == "stop" {
				done <- events
				return
			}
		}
	}()
	require.NoError(t, serve(t.Context(), n, setup))
	require.Equal(t, []string{"install", "setup", "start", "id", "stop"}, <-done)
}

// A failing setup fails the command without starting the node, and stops what Install built.
func TestServe_SetupError(t *testing.T) {
	t.Parallel()
	n := newFakeNode()
	err := serve(t.Context(), n, func() error { return errors.New("boom") })
	require.EqualError(t, err, "boom")
	require.Equal(t, []string{"install", "stop"}, n.drain())
}

func TestServe_InstallError(t *testing.T) {
	t.Parallel()
	n := newFakeNode()
	n.installErr = errors.New("install failed")
	called := false
	require.ErrorContains(t, serve(t.Context(), n, func() error { called = true; return nil }), "install failed")
	require.False(t, called)
	require.Equal(t, []string{"install", "stop"}, n.drain())
}

func TestServe_StartError(t *testing.T) {
	t.Parallel()
	n := newFakeNode()
	n.startErr = errors.New("start failed")
	require.ErrorContains(t, serve(t.Context(), n, nil), "start failed")
	require.Equal(t, []string{"install", "start", "stop"}, n.drain())
}

func TestServe_InvalidProfilerEnv(t *testing.T) { //nolint:paralleltest
	t.Setenv("FSCNODE_PROFILER", "not-a-bool")
	n := newFakeNode()
	n.startErr = errors.New("stop early")
	require.Error(t, serve(t.Context(), n, nil))
}

func TestServe_ProfilerEnabled(t *testing.T) { //nolint:paralleltest
	t.Setenv("FSCNODE_PROFILER", "true")
	t.Setenv("FSCNODE_CFG_PATH", t.TempDir())
	n := newFakeNode()
	n.startErr = errors.New("stop after profiler")
	require.Error(t, serve(t.Context(), n, nil))
}

// A config path that is a regular file makes the profiler's MkdirAll fail, so
// serve() bails out on profiler.Start() before ever reaching n.Install().
func TestServe_ProfilerStartError(t *testing.T) { //nolint:paralleltest
	file := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(file, nil, 0o644))
	t.Setenv("FSCNODE_PROFILER", "true")
	t.Setenv("FSCNODE_CFG_PATH", file)

	n := newFakeNode()
	require.ErrorContains(t, serve(t.Context(), n, nil), "failed to create profile directory")
	require.Empty(t, n.drain())
}

func TestServe_HappyPath(t *testing.T) { //nolint:paralleltest // signals the process
	n := newFakeNode()
	waitThenSignal(t, n)
	require.NoError(t, serve(t.Context(), n, nil))
}

// A second signal while the first Stop still drains calls Stop again, which skips the rest
// of the drain.
func TestServe_SecondSignalSkipsDrain(t *testing.T) { //nolint:paralleltest // signals the process
	n := newFakeNode()
	release := make(chan struct{})
	var calls atomic.Int32
	n.stop = func() {
		if calls.Add(1) == 1 {
			_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
			<-release // the first Stop drains until the second one arrives
			return
		}
		close(release)
	}
	waitThenSignal(t, n)
	require.NoError(t, serve(t.Context(), n, nil))
	require.Equal(t, int32(2), calls.Load())
}

// --check selects checkOnly; --contract and --forget-opener become migrate options.
func TestMigrateCmd(t *testing.T) {
	t.Parallel()
	n := newFakeNode()
	for _, args := range [][]string{{"migrate"}, {"migrate", "--check"}, {"migrate", "--contract", "--forget-opener", "a", "--forget-opener", "b"}} {
		cmd := nodeCmd(n, nil) // cobra keeps flag values between runs
		cmd.SetArgs(args)
		require.NoError(t, cmd.Execute())
	}
	require.Equal(t, []bool{false, true, false}, n.checkOnly)
	require.Equal(t, []int{0, 0, 2}, []int{len(n.opts[0]), len(n.opts[1]), len(n.opts[2])})
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
			n := newFakeNode()
			n.migrateErr = tc.err
			cmd := nodeCmd(n, nil)
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
		n := newFakeNode()
		cmd := nodeCmd(n, nil)
		cmd.SetArgs(args)
		cmd.SetErr(io.Discard)
		var e *ExitError
		require.ErrorAs(t, cmd.Execute(), &e, "%v", args)
		require.Equal(t, ExitUsage, e.Code, "%v", args)
		require.Empty(t, n.checkOnly, "%v called the migrator", args)
	}

	n := newFakeNode()
	cmd := nodeCmd(n, nil)
	cmd.SetArgs([]string{"migrate", "--help"})
	cmd.SetOut(io.Discard)
	require.NoError(t, cmd.Execute())
	require.Empty(t, n.checkOnly)
}
