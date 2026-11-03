/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
	viewsvc "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
)

// mockSDK is a minimal SDK implementation for testing.
type mockSDK struct {
	installErr error
	startErr   error
	installed  bool
	started    bool
}

func (m *mockSDK) Install() error {
	if m.installErr != nil {
		return m.installErr
	}
	m.installed = true
	return nil
}

func (m *mockSDK) Start(_ context.Context) error {
	if m.startErr != nil {
		return m.startErr
	}
	m.started = true
	return nil
}

// mockSDKWithPostStart additionally implements PostStart.
type mockSDKWithPostStart struct {
	mockSDK
	postStartErr error
	postStarted  bool
}

func (m *mockSDKWithPostStart) PostStart(_ context.Context) error {
	if m.postStartErr != nil {
		return m.postStartErr
	}
	m.postStarted = true
	return nil
}

// panicSDK panics during Install to test panic recovery.
type panicSDK struct{}

func (*panicSDK) Install() error                { panic("test panic") }
func (*panicSDK) Start(_ context.Context) error { return nil }

func newTestNode() *Node {
	return &Node{
		sdks:     []SDK{},
		registry: viewsvc.NewServiceProvider(),
		id:       "test-node",
	}
}

// storeOpenerSDK registers a StoreOpener on Install that records the
// configured skipCreateTable of persistence "mydb" when it is called.
type storeOpenerSDK struct {
	mockSDK
	n       *Node
	skipped []bool
	err     error
}

func (s *storeOpenerSDK) Install() error {
	return s.n.RegisterService(storeOpenerFunc(func(context.Context) error {
		var opts struct{ SkipCreateTable bool }
		if err := s.n.config.(*config.Provider).UnmarshalKey("fsc.persistences.mydb.opts", &opts); err != nil {
			return err
		}
		s.skipped = append(s.skipped, opts.SkipCreateTable)
		return s.err
	}))
}

type storeOpenerFunc func(context.Context) error

func (f storeOpenerFunc) OpenStores(ctx context.Context) error { return f(ctx) }

// Migrate installs every SDK without starting it and calls the registered
// StoreOpener with skipCreateTable forced to checkOnly; the opener's error is
// returned.
func TestNode_Migrate(t *testing.T) {
	t.Parallel()

	for _, checkOnly := range []bool{false, true} {
		n, err := NewFromConfPathE(filepath.Join("..", "..", "platform", "view", "services", "config", "testdata"))
		require.NoError(t, err)
		sdk := &storeOpenerSDK{n: n}
		n.AddSDK(sdk)
		require.NoError(t, n.Migrate(t.Context(), checkOnly))
		require.Equal(t, []bool{checkOnly}, sdk.skipped)
		require.False(t, sdk.started)
	}

	n, err := NewFromConfPathE(filepath.Join("..", "..", "platform", "view", "services", "config", "testdata"))
	require.NoError(t, err)
	n.AddSDK(&storeOpenerSDK{n: n, err: errors.New("pending")})
	require.ErrorContains(t, n.Migrate(t.Context(), true), "pending")
}

// An SDK that fails Install after registering the StoreOpener does not hide the
// other stores: Migrate still opens them and joins both errors.
func TestNode_Migrate_InstallFails(t *testing.T) {
	t.Parallel()
	n, err := NewFromConfPathE(filepath.Join("..", "..", "platform", "view", "services", "config", "testdata"))
	require.NoError(t, err)
	installErr, openErr := errors.New("kvs pending"), errors.New("vault pending")
	n.AddSDK(&failingInstallSDK{storeOpenerSDK{n: n, err: openErr}, installErr})
	err = n.Migrate(t.Context(), true)
	require.ErrorIs(t, err, installErr)
	require.ErrorIs(t, err, openErr)
}

type failingInstallSDK struct {
	storeOpenerSDK
	err error
}

func (s *failingInstallSDK) Install() error {
	if err := s.storeOpenerSDK.Install(); err != nil {
		return err
	}
	return s.err
}

func TestNode_Migrate_NoStoreOpener(t *testing.T) {
	t.Parallel()
	n, err := NewFromConfPathE(filepath.Join("..", "..", "platform", "view", "services", "config", "testdata"))
	require.NoError(t, err)
	require.ErrorContains(t, n.Migrate(t.Context(), false), "no store opener registered")
}

func TestNode_ID(t *testing.T) {
	t.Parallel()
	n := newTestNode()
	require.Equal(t, "test-node", n.ID())
}

func TestNode_AddSDK(t *testing.T) {
	t.Parallel()
	n := newTestNode()
	n.AddSDK(&mockSDK{})
	require.Len(t, n.sdks, 1)
}

func TestNode_AddSDK_Multiple(t *testing.T) {
	t.Parallel()
	n := newTestNode()
	n.AddSDK(&mockSDK{})
	n.AddSDK(&mockSDK{})
	require.Len(t, n.sdks, 2)
}

func TestNode_Start_Success(t *testing.T) {
	t.Parallel()
	n := newTestNode()
	sdk := &mockSDK{}
	n.AddSDK(sdk)
	require.NoError(t, n.Start())
	require.True(t, sdk.installed)
	require.True(t, sdk.started)
}

func TestNode_Start_NoSDKs(t *testing.T) {
	t.Parallel()
	n := newTestNode()
	require.NoError(t, n.Start())
}

func TestNode_Start_InstallError(t *testing.T) {
	t.Parallel()
	n := newTestNode()
	n.AddSDK(&mockSDK{installErr: errors.New("install failed")})
	require.Error(t, n.Start())
	require.False(t, n.running)
}

func TestNode_Start_StartError(t *testing.T) {
	t.Parallel()
	n := newTestNode()
	n.AddSDK(&mockSDK{startErr: errors.New("start failed")})
	require.Error(t, n.Start())
	require.False(t, n.running)
}

func TestNode_Start_PostStart(t *testing.T) {
	t.Parallel()
	n := newTestNode()
	sdk := &mockSDKWithPostStart{}
	n.AddSDK(sdk)
	require.NoError(t, n.Start())
	require.True(t, sdk.postStarted)
}

func TestNode_Start_PostStartError(t *testing.T) {
	t.Parallel()
	n := newTestNode()
	n.AddSDK(&mockSDKWithPostStart{postStartErr: errors.New("post-start failed")})
	require.Error(t, n.Start())
}

func TestNode_Start_PanicRecovery(t *testing.T) {
	t.Parallel()
	n := newTestNode()
	n.AddSDK(&panicSDK{})
	err := n.Start()
	require.Error(t, err)
	require.Contains(t, err.Error(), "start triggered panic")
}

func TestNode_Stop(t *testing.T) {
	t.Parallel()
	n := newTestNode()
	require.NoError(t, n.Start())
	require.True(t, n.running)
	n.Stop()
	require.False(t, n.running)
}

func TestNode_Stop_BeforeStart(t *testing.T) {
	t.Parallel()
	n := newTestNode()
	// Stop without Start must not panic (cancel is nil)
	require.NotPanics(t, func() { n.Stop() })
	require.False(t, n.running)
}

func TestNode_InstallSDK_WhenNotRunning(t *testing.T) {
	t.Parallel()
	n := newTestNode()
	require.NoError(t, n.InstallSDK(&mockSDK{}))
	require.Len(t, n.sdks, 1)
}

func TestNode_InstallSDK_WhenRunning(t *testing.T) {
	t.Parallel()
	n := newTestNode()
	require.NoError(t, n.Start())
	err := n.InstallSDK(&mockSDK{})
	require.Error(t, err)
}

func TestNode_RegisterAndGetService(t *testing.T) {
	t.Parallel()
	n := newTestNode()
	svc := &mockSDK{}
	require.NoError(t, n.RegisterService(svc))
	got, err := n.GetService((*mockSDK)(nil))
	require.NoError(t, err)
	require.Equal(t, svc, got)
}

func TestNode_GetService_NotFound(t *testing.T) {
	t.Parallel()
	n := newTestNode()
	_, err := n.GetService((*mockSDK)(nil))
	require.Error(t, err)
}

func TestNewFromConfPathE(t *testing.T) {
	t.Parallel()

	n, err := NewFromConfPathE(filepath.Join("..", "..", "platform", "view", "services", "config", "testdata"))
	require.NoError(t, err)
	require.NotNil(t, n)
}

func TestNewFromConfPathE_InvalidPath(t *testing.T) {
	t.Parallel()

	_, err := NewFromConfPathE("./does-not-exist")
	require.Error(t, err)
}
