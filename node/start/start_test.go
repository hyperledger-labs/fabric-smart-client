/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// package start (internal) to access unexported serve and startCmd functions.
package start

import (
	"errors"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

type mockStartNode struct {
	startErr error
	ch       chan error
}

func (m *mockStartNode) ID() string             { return "test-id" }
func (m *mockStartNode) Start() error           { return m.startErr }
func (m *mockStartNode) Stop()                  {}
func (m *mockStartNode) Callback() chan<- error { return m.ch }

func newMockStartNode(startErr error) *mockStartNode {
	return &mockStartNode{ch: make(chan error, 1), startErr: startErr}
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

func TestServe_HappyPath(t *testing.T) {
	t.Parallel()
	n := newMockStartNode(nil)
	// Once serve() sends nil on the callback channel we know Start() succeeded
	// and it is blocked on ctx.Done(). Send SIGTERM so signal.NotifyContext
	// cancels the context, letting serve() return cleanly.
	go func() {
		select {
		case <-n.ch:
			_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
		case <-t.Context().Done():
		}
	}()
	require.NoError(t, serve(n))
}
