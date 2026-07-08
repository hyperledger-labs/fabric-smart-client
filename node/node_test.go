/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// package node (internal) to access unexported newWithFSCNode, listen, and struct fields.
package node

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	pkgnode "github.com/hyperledger-labs/fabric-smart-client/pkg/node"
)

type mockFSCNode struct{}

func (m *mockFSCNode) ID() string                        { return "test-id" }
func (m *mockFSCNode) Start() error                      { return nil }
func (m *mockFSCNode) Stop()                             {}
func (m *mockFSCNode) InstallSDK(p pkgnode.SDK) error    { return nil }
func (m *mockFSCNode) GetService(v any) (any, error)     { return nil, nil }
func (m *mockFSCNode) RegisterService(service any) error { return nil }

func writeCoreYAML(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "core.yaml"), []byte(""), 0o644))
	return dir
}

func TestNewWithConfPath(t *testing.T) {
	t.Parallel()
	dir := writeCoreYAML(t)
	n := NewWithConfPath(dir)
	require.NotNil(t, n)
}

func TestNew(t *testing.T) { //nolint:paralleltest
	dir := writeCoreYAML(t)
	t.Setenv("FSCNODE_CFG_PATH", dir)
	n := New()
	require.NotNil(t, n)
}

func TestExecute_VersionCmd(t *testing.T) { //nolint:paralleltest
	origArgs := os.Args
	t.Cleanup(func() { os.Args = origArgs })

	n := newWithFSCNode(&mockFSCNode{})
	os.Args = []string{"peer", "version"}
	n.Execute(nil)
	// Unblock the listen goroutine started by Execute.
	n.callbackChannel <- nil
}

func TestNewWithFSCNode_Commands(t *testing.T) {
	t.Parallel()
	n := newWithFSCNode(&mockFSCNode{})
	require.NotNil(t, n)
	names := make(map[string]bool)
	for _, cmd := range n.mainCmd.Commands() {
		names[cmd.Use] = true
	}
	require.True(t, names["version"], "version subcommand should be registered")
	require.True(t, names["node"], "node subcommand should be registered")
}

func TestCallback(t *testing.T) {
	t.Parallel()
	n := newWithFSCNode(&mockFSCNode{})
	ch := n.Callback()
	require.NotNil(t, ch)
}

func TestListen_NilError_NoCallback(t *testing.T) {
	t.Parallel()
	n := newWithFSCNode(&mockFSCNode{})
	done := make(chan struct{})
	go func() {
		n.listen()
		close(done)
	}()
	n.callbackChannel <- nil
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	t.Cleanup(cancel)
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("listen did not return after nil error with no callback set")
	}
}

func TestListen_NilError_WithCallback(t *testing.T) {
	t.Parallel()
	n := newWithFSCNode(&mockFSCNode{})
	called := make(chan struct{})
	n.executeCallbackFunc = func() error {
		close(called)
		return nil
	}
	go n.listen()
	n.callbackChannel <- nil
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	t.Cleanup(cancel)
	select {
	case <-called:
	case <-ctx.Done():
		t.Fatal("executeCallbackFunc was not called")
	}
}
