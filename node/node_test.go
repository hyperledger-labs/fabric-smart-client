/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// package node (internal) to access unexported newWithFSCNode, listen, and struct fields.
package node

import (
	"errors"
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

// runListen runs listen in a goroutine, feeds err on the callback channel and
// returns whatever listen panicked with (nil if it returned normally).
func runListen(t *testing.T, n *Node, err error) any {
	t.Helper()
	got := make(chan any, 1)
	go func() {
		defer func() { got <- recover() }()
		n.listen()
	}()
	n.callbackChannel <- err
	select {
	case r := <-got:
		return r
	case <-time.After(time.Second):
		t.Fatal("listen did not finish")
		return nil
	}
}

func TestListen_NilError_NoCallback(t *testing.T) {
	t.Parallel()
	n := newWithFSCNode(&mockFSCNode{})
	require.Nil(t, runListen(t, n, nil), "listen should return without panicking")
}

func TestListen_NilError_WithCallback(t *testing.T) {
	t.Parallel()
	n := newWithFSCNode(&mockFSCNode{})
	called := false
	n.executeCallbackFunc = func() error {
		called = true
		return nil
	}
	require.Nil(t, runListen(t, n, nil), "listen should return without panicking")
	require.True(t, called, "executeCallbackFunc was not called")
}

func TestListen_ChannelErrorPanics(t *testing.T) {
	t.Parallel()
	n := newWithFSCNode(&mockFSCNode{})
	err, ok := runListen(t, n, errors.New("boom")).(error)
	require.True(t, ok, "listen should panic with the channel error")
	require.EqualError(t, err, "boom")
}

func TestListen_CallbackErrorPanics(t *testing.T) {
	t.Parallel()
	n := newWithFSCNode(&mockFSCNode{})
	n.executeCallbackFunc = func() error { return errors.New("callback boom") }
	err, ok := runListen(t, n, nil).(error)
	require.True(t, ok, "listen should panic with the callback error")
	require.EqualError(t, err, "callback boom")
}
