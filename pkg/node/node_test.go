/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"context"
	"errors"
	"testing"

	viewsvc "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/stretchr/testify/require"
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

func (p *panicSDK) Install() error                { panic("test panic") }
func (p *panicSDK) Start(_ context.Context) error { return nil }

func newTestNode() *Node {
	return &Node{
		sdks:     []SDK{},
		registry: viewsvc.NewServiceProvider(),
		id:       "test-node",
	}
}

func TestNode_ID(t *testing.T) {
	n := newTestNode()
	require.Equal(t, "test-node", n.ID())
}

func TestNode_AddSDK(t *testing.T) {
	n := newTestNode()
	n.AddSDK(&mockSDK{})
	require.Len(t, n.sdks, 1)
}

func TestNode_AddSDK_Multiple(t *testing.T) {
	n := newTestNode()
	n.AddSDK(&mockSDK{})
	n.AddSDK(&mockSDK{})
	require.Len(t, n.sdks, 2)
}

func TestNode_Start_Success(t *testing.T) {
	n := newTestNode()
	sdk := &mockSDK{}
	n.AddSDK(sdk)
	require.NoError(t, n.Start())
	require.True(t, sdk.installed)
	require.True(t, sdk.started)
}

func TestNode_Start_NoSDKs(t *testing.T) {
	n := newTestNode()
	require.NoError(t, n.Start())
}

func TestNode_Start_InstallError(t *testing.T) {
	n := newTestNode()
	n.AddSDK(&mockSDK{installErr: errors.New("install failed")})
	require.Error(t, n.Start())
}

func TestNode_Start_StartError(t *testing.T) {
	n := newTestNode()
	n.AddSDK(&mockSDK{startErr: errors.New("start failed")})
	require.Error(t, n.Start())
}

func TestNode_Start_PostStart(t *testing.T) {
	n := newTestNode()
	sdk := &mockSDKWithPostStart{}
	n.AddSDK(sdk)
	require.NoError(t, n.Start())
	require.True(t, sdk.postStarted)
}

func TestNode_Start_PostStartError(t *testing.T) {
	n := newTestNode()
	n.AddSDK(&mockSDKWithPostStart{postStartErr: errors.New("post-start failed")})
	require.Error(t, n.Start())
}

func TestNode_Start_PanicRecovery(t *testing.T) {
	n := newTestNode()
	n.AddSDK(&panicSDK{})
	err := n.Start()
	require.Error(t, err)
	require.Contains(t, err.Error(), "start triggered panic")
}

func TestNode_Stop(t *testing.T) {
	n := newTestNode()
	require.NoError(t, n.Start())
	require.True(t, n.running)
	n.Stop()
	require.False(t, n.running)
}

func TestNode_Stop_BeforeStart(t *testing.T) {
	n := newTestNode()
	// Stop without Start must not panic (cancel is nil)
	require.NotPanics(t, func() { n.Stop() })
	require.False(t, n.running)
}

func TestNode_InstallSDK_WhenNotRunning(t *testing.T) {
	n := newTestNode()
	require.NoError(t, n.InstallSDK(&mockSDK{}))
	require.Len(t, n.sdks, 1)
}

func TestNode_InstallSDK_WhenRunning(t *testing.T) {
	n := newTestNode()
	require.NoError(t, n.Start())
	err := n.InstallSDK(&mockSDK{})
	require.Error(t, err)
}

func TestNode_RegisterAndGetService(t *testing.T) {
	n := newTestNode()
	svc := &mockSDK{}
	require.NoError(t, n.RegisterService(svc))
	got, err := n.GetService((*mockSDK)(nil))
	require.NoError(t, err)
	require.Equal(t, svc, got)
}

func TestNode_GetService_NotFound(t *testing.T) {
	n := newTestNode()
	_, err := n.GetService((*mockSDK)(nil))
	require.Error(t, err)
}
