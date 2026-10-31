/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	viewapi "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// A view the node's shutdown cancelled ends the unary call with an Aborted status, not an
// error response, so the client tells it from a failed view.
func TestProcessCommandAborted(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t)
	srv.RegisterProcessor(reflect.TypeFor[*protos.Command_CallView](), func(context.Context, *protos.Command) (any, error) {
		return nil, ErrAbortedByShutdown
	})
	resp, err := srv.ProcessCommand(t.Context(), buildSignedCommand(t, nil))
	require.Nil(t, resp)
	require.Equal(t, codes.Aborted, status.Code(err))
}

func TestViewErr_CancelledByShutdownIsAborted(t *testing.T) {
	t.Parallel()
	err := viewErr(t.Context(), "v", errors.Wrap(view.ErrCancelledByShutdown, "submit"))
	require.Equal(t, codes.Aborted, status.Code(err))
}

// trackingViewManager is a fakeViewManager that counts running views like the view manager.
type trackingViewManager struct {
	fakeViewManager
	running int
}

func (m *trackingViewManager) Track() func() {
	m.running++
	return func() { m.running-- }
}

// A streamed view runs in a context from InitiateContext, which does not track it; the
// handler tracks it, so a draining node waits for it.
func TestStreamCallView_TracksRunningView(t *testing.T) {
	t.Parallel()
	vm := &trackingViewManager{}
	ctx := &trackedContext{vm: vm}
	ctx.id = "c"
	vm.initiateContextReturns = ctx
	srv := &fakeService{}
	InstallViewHandler(vm, srv, time.Minute)
	cmd := &protos.Command{Payload: &protos.Command_CallView{CallView: &protos.CallView{Fid: "v"}}}

	require.NoError(t, srv.streamers[reflect.TypeFor[*protos.Command_CallView]()](&protos.SignedCommand{}, cmd, &viewTestStreamServer{}, &fakeMarshaller{}))
	require.Equal(t, 1, ctx.runningDuringView)
	require.Zero(t, vm.running)
}

// trackedContext records how many views its manager counted while it ran one.
type trackedContext struct {
	fakeContext
	vm                *trackingViewManager
	runningDuringView int
}

func (c *trackedContext) RunView(viewapi.View, ...viewapi.RunViewOption) (any, error) {
	c.runningDuringView = c.vm.running
	return "ok", nil
}
