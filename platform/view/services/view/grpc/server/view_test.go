/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// --- fakes ---

type fakeViewManager struct {
	newViewCallCount         int
	initiateViewCallCount    int
	initiateContextCallCount int
	deleteContextCallCount   int

	newViewReturns         view2.View
	newViewErr             error
	initiateViewReturns    any
	initiateViewErr        error
	initiateContextReturns view2.Context
	initiateContextErr     error

	lastNewViewID string
	lastNewViewIn []byte
}

func (f *fakeViewManager) NewView(id string, in []byte) (view2.View, error) {
	f.newViewCallCount++
	f.lastNewViewID = id
	f.lastNewViewIn = in
	return f.newViewReturns, f.newViewErr
}

func (f *fakeViewManager) InitiateView(_ context.Context, _ view2.View) (any, error) {
	f.initiateViewCallCount++
	return f.initiateViewReturns, f.initiateViewErr
}

func (f *fakeViewManager) InitiateContext(_ context.Context, _ view2.View) (view2.Context, error) {
	f.initiateContextCallCount++
	return f.initiateContextReturns, f.initiateContextErr
}

func (f *fakeViewManager) DeleteContext(_ string) {
	f.deleteContextCallCount++
}

type fakeContext struct {
	id             string
	runViewReturns any
	runViewErr     error
	putServiceErr  error
	services       []any
}

func (f *fakeContext) ID() string { return f.id }
func (f *fakeContext) RunView(view2.View, ...view2.RunViewOption) (any, error) {
	return f.runViewReturns, f.runViewErr
}

func (f *fakeContext) PutService(v any) error {
	if f.putServiceErr != nil {
		return f.putServiceErr
	}
	f.services = append(f.services, v)
	return nil
}

func (*fakeContext) ResetSessions() error     { return nil }
func (*fakeContext) Context() context.Context { return context.Background() }
func (*fakeContext) StartSpanFrom(ctx context.Context, _ string, _ ...trace.SpanStartOption) (context.Context, trace.Span) {
	return ctx, trace.SpanFromContext(ctx)
}
func (*fakeContext) Initiator() view2.View         { return nil }
func (*fakeContext) Me() view2.Identity            { return nil }
func (*fakeContext) IsMe(view2.Identity) bool      { return false }
func (*fakeContext) Session() view2.Session        { return nil }
func (*fakeContext) GetService(_ any) (any, error) { return nil, nil }
func (*fakeContext) GetSession(_ view2.View, _ view2.Identity, _ ...view2.View) (view2.Session, error) {
	return nil, nil
}

func (*fakeContext) GetSessionByID(_ string, _ view2.Identity) (view2.Session, error) {
	return nil, nil
}

func (*fakeContext) StartSession(view2.View, view2.Identity) (view2.Session, error) {
	return nil, nil
}
func (*fakeContext) OnError(func()) {}

// fakeNonMutableContext implements view2.Context but deliberately not
// view2.MutableContext, to exercise the mutable-context guard.
type fakeNonMutableContext struct {
	// embedded under an alias so the promoted Context() method is not
	// shadowed by a field named Context; only ID() is ever called.
	viewContext
	id string
}

// viewContext aliases view2.Context so the embedded field above is not named
// Context, which would shadow the interface's own Context() method.
type viewContext interface{ view2.Context }

func (f *fakeNonMutableContext) ID() string { return f.id }

type fakeMarshaller struct {
	marshalCommandResponseCallCount int
	marshalCommandResponseReturns   *protos.SignedCommandResponse
	marshalCommandResponseErr       error

	lastCommand         []byte
	lastResponsePayload any
}

func (f *fakeMarshaller) MarshalCommandResponse(command []byte, responsePayload any) (*protos.SignedCommandResponse, error) {
	f.marshalCommandResponseCallCount++
	f.lastCommand = command
	f.lastResponsePayload = responsePayload
	return f.marshalCommandResponseReturns, f.marshalCommandResponseErr
}

// lastCallViewResult returns the Result the handler placed in the marshalled
// response, failing the test if no CallViewResponse was marshalled.
func (f *fakeMarshaller) lastCallViewResult(t *testing.T) []byte {
	t.Helper()
	payload, ok := f.lastResponsePayload.(*protos.CommandResponse_CallViewResponse)
	require.True(t, ok, "expected a CallViewResponse payload, got %T", f.lastResponsePayload)
	return payload.CallViewResponse.Result
}

type fakeService struct {
	protos.UnimplementedViewServiceServer
	processors map[reflect.Type]Processor
	streamers  map[reflect.Type]Streamer
}

func (f *fakeService) RegisterProcessor(typ reflect.Type, p Processor) {
	if f.processors == nil {
		f.processors = make(map[reflect.Type]Processor)
	}
	f.processors[typ] = p
}

func (f *fakeService) RegisterStreamer(typ reflect.Type, streamer Streamer) {
	if f.streamers == nil {
		f.streamers = make(map[reflect.Type]Streamer)
	}
	f.streamers[typ] = streamer
}

type viewTestStreamServer struct {
	protos.ViewService_StreamCommandServer
	ctx      context.Context
	sentMsgs []any
}

func (f *viewTestStreamServer) Context() context.Context {
	if f.ctx == nil {
		return context.Background()
	}
	return f.ctx
}

func (f *viewTestStreamServer) Send(r *protos.SignedCommandResponse) error {
	f.sentMsgs = append(f.sentMsgs, r)
	return nil
}

func (f *viewTestStreamServer) SendMsg(m any) error {
	f.sentMsgs = append(f.sentMsgs, m)
	return nil
}

// --- tests ---

func TestInstallViewHandler(t *testing.T) {
	t.Parallel()
	vm := &fakeViewManager{}
	srv := &fakeService{}

	InstallViewHandler(vm, srv, 0)

	// CallView is the only command; any other payload gets the server's "command type not
	// recognized" response.
	require.Len(t, srv.processors, 1)
	require.Len(t, srv.streamers, 1)

	// Each handler is invoked rather than only checked for non-nil: asserting
	// NotNil alone still passes if the registrations are swapped.
	callProcessor := srv.processors[reflect.TypeFor[*protos.Command_CallView]()]
	require.NotNil(t, callProcessor)
	result, err := callProcessor(context.Background(), &protos.Command{
		Payload: &protos.Command_CallView{
			CallView: &protos.CallView{Fid: "call_fid"},
		},
	})
	require.NoError(t, err)
	require.IsType(t, &protos.CommandResponse_CallViewResponse{}, result)
	require.Equal(t, "call_fid", vm.lastNewViewID)

	streamer := srv.streamers[reflect.TypeFor[*protos.Command_CallView]()]
	require.NotNil(t, streamer)
	vm.initiateContextReturns = &fakeContext{id: "stream_ctx"}
	scs := &viewTestStreamServer{}
	marshaller := &fakeMarshaller{marshalCommandResponseReturns: &protos.SignedCommandResponse{}}
	require.NoError(t, streamer(
		&protos.SignedCommand{Command: []byte("signed_command_bytes")},
		&protos.Command{
			Payload: &protos.Command_CallView{
				CallView: &protos.CallView{Fid: "stream_fid"},
			},
		},
		scs,
		marshaller,
	))
	require.Equal(t, "stream_fid", vm.lastNewViewID)
	require.Len(t, scs.sentMsgs, 1)
}

func TestCallView(t *testing.T) {
	t.Parallel()
	vm := &fakeViewManager{}

	vh := &viewHandler{
		viewManager: vm,
	}

	command := &protos.Command{
		Payload: &protos.Command_CallView{
			CallView: &protos.CallView{
				Fid:   "test_view",
				Input: []byte("test_input"),
			},
		},
	}

	expectedResult := []byte("test_result")
	vm.initiateViewReturns = expectedResult

	result, err := vh.callView(context.Background(), command)
	require.NoError(t, err)

	resp, ok := result.(*protos.CommandResponse_CallViewResponse)
	require.True(t, ok)
	require.Equal(t, expectedResult, resp.CallViewResponse.Result)

	require.Equal(t, 1, vm.newViewCallCount)
	require.Equal(t, 1, vm.initiateViewCallCount)
}

func TestCallView_Error(t *testing.T) {
	t.Parallel()
	command := &protos.Command{
		Payload: &protos.Command_CallView{
			CallView: &protos.CallView{Fid: "test_view", Input: []byte("test_input")},
		},
	}

	t.Run("NewView fails", func(t *testing.T) {
		t.Parallel()
		vm := &fakeViewManager{newViewErr: errors.New("new_view_error")}
		vh := &viewHandler{viewManager: vm}

		_, err := vh.callView(context.Background(), command)
		require.Error(t, err)
		require.Contains(t, err.Error(), "new_view_error")
	})

	t.Run("InitiateView fails", func(t *testing.T) {
		t.Parallel()
		vm := &fakeViewManager{initiateViewErr: errors.New("initiate_view_error")}
		vh := &viewHandler{viewManager: vm}

		_, err := vh.callView(context.Background(), command)
		require.Error(t, err)
		require.Contains(t, err.Error(), "initiate_view_error")
	})
}

func TestStreamCallView(t *testing.T) {
	t.Parallel()
	vm := &fakeViewManager{}
	marshaller := &fakeMarshaller{}

	vh := &viewHandler{
		viewManager: vm,
	}

	command := &protos.Command{
		Payload: &protos.Command_CallView{
			CallView: &protos.CallView{
				Fid:   "test_view",
				Input: []byte("test_input"),
			},
		},
	}
	sc := &protos.SignedCommand{Command: []byte("signed_command_bytes")}
	scs := &viewTestStreamServer{}

	mockCtx := &fakeContext{
		id:             "test_context_id",
		runViewReturns: []byte("test_result"),
	}
	vm.initiateContextReturns = mockCtx
	marshaller.marshalCommandResponseReturns = &protos.SignedCommandResponse{}

	err := vh.streamCallView(sc, command, scs, marshaller)
	require.NoError(t, err)

	require.Equal(t, 1, vm.newViewCallCount)
	require.Equal(t, 1, vm.initiateContextCallCount)
	require.Len(t, mockCtx.services, 1)
	require.Equal(t, 1, marshaller.marshalCommandResponseCallCount)
	require.Equal(t, sc.Command, marshaller.lastCommand)
	require.Equal(t, []byte("test_result"), marshaller.lastCallViewResult(t))
	require.Len(t, scs.sentMsgs, 1)
}

func TestStreamCallView_Error(t *testing.T) {
	t.Parallel()
	command := &protos.Command{
		Payload: &protos.Command_CallView{
			CallView: &protos.CallView{Fid: "test_view", Input: []byte("test_input")},
		},
	}
	sc := &protos.SignedCommand{Command: []byte("signed_command_bytes")}

	t.Run("NewView fails", func(t *testing.T) {
		t.Parallel()
		vm := &fakeViewManager{newViewErr: errors.New("new_view_error")}
		vh := &viewHandler{viewManager: vm}
		marshaller := &fakeMarshaller{}

		err := vh.streamCallView(sc, command, &viewTestStreamServer{}, marshaller)
		require.Error(t, err)
		require.Contains(t, err.Error(), "new_view_error")
	})

	t.Run("InitiateContext fails", func(t *testing.T) {
		t.Parallel()
		vm := &fakeViewManager{initiateContextErr: errors.New("initiate_context_error")}
		vh := &viewHandler{viewManager: vm}
		marshaller := &fakeMarshaller{}

		err := vh.streamCallView(sc, command, &viewTestStreamServer{}, marshaller)
		require.Error(t, err)
		require.Contains(t, err.Error(), "initiate_context_error")
	})

	t.Run("PutService fails", func(t *testing.T) {
		t.Parallel()
		mockCtx := &fakeContext{id: "ctx", putServiceErr: errors.New("put_service_error")}
		vm := &fakeViewManager{initiateContextReturns: mockCtx}
		vh := &viewHandler{viewManager: vm}
		marshaller := &fakeMarshaller{}

		err := vh.streamCallView(sc, command, &viewTestStreamServer{}, marshaller)
		require.Error(t, err)
		require.Contains(t, err.Error(), "put_service_error")
		require.Equal(t, 1, vm.deleteContextCallCount)
	})

	t.Run("context is not mutable", func(t *testing.T) {
		t.Parallel()
		vm := &fakeViewManager{initiateContextReturns: &fakeNonMutableContext{id: "ctx"}}
		vh := &viewHandler{viewManager: vm}
		marshaller := &fakeMarshaller{}

		err := vh.streamCallView(sc, command, &viewTestStreamServer{}, marshaller)
		require.Error(t, err)
		require.Contains(t, err.Error(), "expected a mutable context")
		require.Equal(t, 1, vm.deleteContextCallCount)
	})

	t.Run("RunView fails", func(t *testing.T) {
		t.Parallel()
		mockCtx := &fakeContext{id: "ctx", runViewErr: errors.New("run_view_error")}
		vm := &fakeViewManager{initiateContextReturns: mockCtx}
		vh := &viewHandler{viewManager: vm}
		marshaller := &fakeMarshaller{}

		err := vh.streamCallView(sc, command, &viewTestStreamServer{}, marshaller)
		require.Error(t, err)
		require.Contains(t, err.Error(), "run_view_error")
	})
}

func TestCallView_JSON(t *testing.T) {
	t.Parallel()
	command := &protos.Command{
		Payload: &protos.Command_CallView{
			CallView: &protos.CallView{Fid: "test_view", Input: []byte("test_input")},
		},
	}

	t.Run("non-byte result is JSON marshalled", func(t *testing.T) {
		t.Parallel()
		vm := &fakeViewManager{initiateViewReturns: map[string]string{"key": "value"}}
		vh := &viewHandler{viewManager: vm}

		result, err := vh.callView(context.Background(), command)
		require.NoError(t, err)
		resp, ok := result.(*protos.CommandResponse_CallViewResponse)
		require.True(t, ok)
		require.Contains(t, string(resp.CallViewResponse.Result), "value")
	})

	t.Run("unmarshalable result returns error", func(t *testing.T) {
		t.Parallel()
		vm := &fakeViewManager{initiateViewReturns: make(chan int)}
		vh := &viewHandler{viewManager: vm}

		_, err := vh.callView(context.Background(), command)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed marshalling result")
	})
}

func TestStreamCallView_JSON(t *testing.T) {
	t.Parallel()
	command := &protos.Command{
		Payload: &protos.Command_CallView{
			CallView: &protos.CallView{Fid: "test_view", Input: []byte("test_input")},
		},
	}
	sc := &protos.SignedCommand{Command: []byte("signed_command_bytes")}

	t.Run("non-byte result is JSON marshalled", func(t *testing.T) {
		t.Parallel()
		mockCtx := &fakeContext{id: "ctx", runViewReturns: map[string]string{"key": "value"}}
		vm := &fakeViewManager{initiateContextReturns: mockCtx}
		vh := &viewHandler{viewManager: vm}
		marshaller := &fakeMarshaller{marshalCommandResponseReturns: &protos.SignedCommandResponse{}}

		err := vh.streamCallView(sc, command, &viewTestStreamServer{}, marshaller)
		require.NoError(t, err)
		require.Equal(t, sc.Command, marshaller.lastCommand)
		require.Contains(t, string(marshaller.lastCallViewResult(t)), "value")
	})

	t.Run("unmarshalable result returns error", func(t *testing.T) {
		t.Parallel()
		mockCtx := &fakeContext{id: "ctx", runViewReturns: make(chan int)}
		vm := &fakeViewManager{initiateContextReturns: mockCtx}
		vh := &viewHandler{viewManager: vm}
		marshaller := &fakeMarshaller{}

		err := vh.streamCallView(sc, command, &viewTestStreamServer{}, marshaller)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed marshalling result")
	})
}

type streamRecvSendServer struct {
	protos.ViewService_StreamCommandServer
	recvMsg any
	sendMsg any
}

func (d *streamRecvSendServer) SendMsg(m any) error { d.sendMsg = m; return nil }

func (d *streamRecvSendServer) RecvMsg(m any) error {
	if d.recvMsg == nil {
		return errors.New("recv_error")
	}
	bytes, _ := json.Marshal(d.recvMsg)
	resp, ok := m.(*protos.CallViewResponse)
	if !ok {
		return errors.Errorf("unexpected type %T", m)
	}
	resp.Result = bytes
	return nil
}

func TestStream(t *testing.T) {
	t.Parallel()
	t.Run("Send marshals and forwards", func(t *testing.T) {
		t.Parallel()
		scs := &streamRecvSendServer{}
		stream := &Stream{scs: scs}

		err := stream.Send(map[string]string{"msg": "hello"})
		require.NoError(t, err)
		require.NotNil(t, scs.sendMsg)
		sentResp, ok := scs.sendMsg.(*protos.CallViewResponse)
		require.True(t, ok)
		require.Contains(t, string(sentResp.Result), "hello")
	})

	t.Run("Send returns error for unmarshalable type", func(t *testing.T) {
		t.Parallel()
		scs := &streamRecvSendServer{}
		stream := &Stream{scs: scs}

		err := stream.Send(make(chan int))
		require.Error(t, err)
	})

	t.Run("Recv unmarshals response", func(t *testing.T) {
		t.Parallel()
		scs := &streamRecvSendServer{recvMsg: map[string]string{"msg": "world"}}
		stream := &Stream{scs: scs}

		var msgToRecv map[string]string
		err := stream.Recv(&msgToRecv)
		require.NoError(t, err)
		require.Equal(t, "world", msgToRecv["msg"])
	})

	t.Run("Recv returns error on transport failure", func(t *testing.T) {
		t.Parallel()
		scs := &streamRecvSendServer{recvMsg: nil}
		stream := &Stream{scs: scs}

		var msgToRecv map[string]string
		err := stream.Recv(&msgToRecv)
		require.Error(t, err)
		require.Contains(t, err.Error(), "recv_error")
	})
}

// deadlineViewManager runs every view until its context is done, then returns err, or the
// context's error when err is nil.
type deadlineViewManager struct {
	fakeViewManager
	err error
}

func (*deadlineViewManager) NewView(string, []byte) (view2.View, error) { return nil, nil }

func (m *deadlineViewManager) InitiateView(ctx context.Context, _ view2.View) (any, error) {
	<-ctx.Done()
	if m.err != nil {
		return nil, m.err
	}
	return nil, ctx.Err()
}

func (*deadlineViewManager) InitiateContext(ctx context.Context, _ view2.View) (view2.Context, error) {
	return &deadlineContext{id: "ctx", ctx: ctx}, nil
}

// deadlineContext runs a view until ctx is done.
type deadlineContext struct {
	fakeContext
	ctx context.Context
}

func (c *deadlineContext) RunView(view2.View, ...view2.RunViewOption) (any, error) {
	<-c.ctx.Done()
	return nil, c.ctx.Err()
}

// A view that outlives the timeout ends with ErrViewTimedOut on both the unary and the
// streamed path, and nothing of the view's own error reaches the client.
func TestViewHandlerTimesOut(t *testing.T) {
	t.Parallel()
	srv := &fakeService{}
	InstallViewHandler(&deadlineViewManager{}, srv, 20*time.Millisecond)
	cmd := &protos.Command{Payload: &protos.Command_CallView{CallView: &protos.CallView{Fid: "slow"}}}

	_, err := srv.processors[reflect.TypeFor[*protos.Command_CallView]()](t.Context(), cmd)
	require.ErrorIs(t, err, ErrViewTimedOut)
	require.EqualError(t, err, "view execution timed out")

	err = srv.streamers[reflect.TypeFor[*protos.Command_CallView]()](&protos.SignedCommand{}, cmd, &viewTestStreamServer{}, &fakeMarshaller{})
	require.EqualError(t, err, "view execution timed out")
}

// Only the handler's own deadline is a timeout: a view that fails with a deadline error of
// its own, while the handler's context is still live, reports that failure.
func TestViewHandlerReportsTheViewsOwnDeadlineError(t *testing.T) {
	t.Parallel()
	srv := &fakeService{}
	ctx, cancel := context.WithCancel(t.Context())
	vm := &deadlineViewManager{err: errors.Wrap(context.DeadlineExceeded, "dial peer")}
	InstallViewHandler(vm, srv, time.Hour)
	cmd := &protos.Command{Payload: &protos.Command_CallView{CallView: &protos.CallView{Fid: "v"}}}
	cancel() // the client went away; the view returns its own deadline error

	_, err := srv.processors[reflect.TypeFor[*protos.Command_CallView]()](ctx, cmd)
	require.ErrorIs(t, err, view.ErrViewExecutionFailed)
	require.NotErrorIs(t, err, ErrViewTimedOut)
}
