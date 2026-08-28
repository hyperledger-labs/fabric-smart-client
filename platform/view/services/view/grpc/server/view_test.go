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

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
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

func (f *fakeViewManager) InitiateView(ctx context.Context, view view2.View) (any, error) {
	f.initiateViewCallCount++
	return f.initiateViewReturns, f.initiateViewErr
}

func (f *fakeViewManager) InitiateContext(ctx context.Context, view view2.View) (view2.Context, error) {
	f.initiateContextCallCount++
	return f.initiateContextReturns, f.initiateContextErr
}

func (f *fakeViewManager) DeleteContext(contextID string) {
	f.deleteContextCallCount++
}

type fakeContext struct {
	id             string
	runViewReturns any
	runViewErr     error
	services       []any
}

func (f *fakeContext) ID() string { return f.id }
func (f *fakeContext) RunView(view2.View, ...view2.RunViewOption) (any, error) {
	return f.runViewReturns, f.runViewErr
}

func (f *fakeContext) PutService(v any) error {
	f.services = append(f.services, v)
	return nil
}
func (f *fakeContext) ResetSessions() error     { return nil }
func (f *fakeContext) Context() context.Context { return context.Background() }
func (f *fakeContext) StartSpanFrom(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return ctx, trace.SpanFromContext(ctx)
}
func (f *fakeContext) Initiator() view2.View         { return nil }
func (f *fakeContext) Me() view2.Identity            { return nil }
func (f *fakeContext) IsMe(view2.Identity) bool      { return false }
func (f *fakeContext) Session() view2.Session        { return nil }
func (f *fakeContext) GetService(v any) (any, error) { return nil, nil }
func (f *fakeContext) GetSession(caller view2.View, p view2.Identity, views ...view2.View) (view2.Session, error) {
	return nil, nil
}

func (f *fakeContext) GetSessionByID(id string, p view2.Identity) (view2.Session, error) {
	return nil, nil
}

func (f *fakeContext) StartSession(view2.View, view2.Identity) (view2.Session, error) {
	return nil, nil
}
func (f *fakeContext) OnError(func()) {}

type fakeMarshaller struct {
	marshalCommandResponseCallCount int
	marshalCommandResponseReturns   *protos.SignedCommandResponse
	marshalCommandResponseErr       error
}

func (f *fakeMarshaller) MarshalCommandResponse(command []byte, responsePayload any) (*protos.SignedCommandResponse, error) {
	f.marshalCommandResponseCallCount++
	return f.marshalCommandResponseReturns, f.marshalCommandResponseErr
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

type fakeTracerProvider struct {
	trace.TracerProvider
}

func (f *fakeTracerProvider) Tracer(name string, opts ...trace.TracerOption) trace.Tracer {
	return noop.NewTracerProvider().Tracer(name)
}

// --- tests ---

func TestInstallViewHandler(t *testing.T) {
	t.Parallel()
	vm := &fakeViewManager{}
	srv := &fakeService{}
	tp := &fakeTracerProvider{}

	InstallViewHandler(vm, srv, tp)

	require.Equal(t, 2, len(srv.processors))
	require.Equal(t, 1, len(srv.streamers))

	require.NotNil(t, srv.processors[reflect.TypeFor[*protos.Command_InitiateView]()])
	require.NotNil(t, srv.processors[reflect.TypeFor[*protos.Command_CallView]()])
	require.NotNil(t, srv.streamers[reflect.TypeFor[*protos.Command_CallView]()])
}

func TestInitiateView(t *testing.T) {
	t.Parallel()
	vm := &fakeViewManager{}
	tp := &fakeTracerProvider{}

	vh := &viewHandler{
		viewManager: vm,
		tracer:      tp.Tracer("view_handler"),
	}

	command := &protos.Command{
		Payload: &protos.Command_InitiateView{
			InitiateView: &protos.InitiateView{
				Fid:   "test_view",
				Input: []byte("test_input"),
			},
		},
	}

	mockCtx := &fakeContext{id: "test_context_id"}
	vm.initiateContextReturns = mockCtx

	result, err := vh.initiateView(context.Background(), command)
	require.NoError(t, err)

	resp, ok := result.(*protos.CommandResponse_InitiateViewResponse)
	require.True(t, ok)
	require.Equal(t, "test_context_id", resp.InitiateViewResponse.Cid)

	require.Equal(t, 1, vm.newViewCallCount)
	require.Equal(t, "test_view", vm.lastNewViewID)
	require.Equal(t, []byte("test_input"), vm.lastNewViewIn)

	require.Equal(t, 1, vm.initiateContextCallCount)
}

func TestInitiateView_Error(t *testing.T) {
	t.Parallel()
	t.Run("NewView fails", func(t *testing.T) {
		t.Parallel()
		vm := &fakeViewManager{newViewErr: errors.New("new_view_error")}
		tp := &fakeTracerProvider{}
		vh := &viewHandler{viewManager: vm, tracer: tp.Tracer("view_handler")}

		command := &protos.Command{
			Payload: &protos.Command_InitiateView{
				InitiateView: &protos.InitiateView{Fid: "test_view", Input: []byte("test_input")},
			},
		}

		_, err := vh.initiateView(context.Background(), command)
		require.Error(t, err)
		require.Contains(t, err.Error(), "new_view_error")
	})

	t.Run("InitiateContext fails", func(t *testing.T) {
		t.Parallel()
		vm := &fakeViewManager{initiateContextErr: errors.New("initiate_context_error")}
		tp := &fakeTracerProvider{}
		vh := &viewHandler{viewManager: vm, tracer: tp.Tracer("view_handler")}

		command := &protos.Command{
			Payload: &protos.Command_InitiateView{
				InitiateView: &protos.InitiateView{Fid: "test_view", Input: []byte("test_input")},
			},
		}

		_, err := vh.initiateView(context.Background(), command)
		require.Error(t, err)
		require.Contains(t, err.Error(), "initiate_context_error")
	})
}

func TestCallView(t *testing.T) {
	t.Parallel()
	vm := &fakeViewManager{}
	tp := &fakeTracerProvider{}

	vh := &viewHandler{
		viewManager: vm,
		tracer:      tp.Tracer("view_handler"),
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
		tp := &fakeTracerProvider{}
		vh := &viewHandler{viewManager: vm, tracer: tp.Tracer("view_handler")}

		_, err := vh.callView(context.Background(), command)
		require.Error(t, err)
		require.Contains(t, err.Error(), "new_view_error")
	})

	t.Run("InitiateView fails", func(t *testing.T) {
		t.Parallel()
		vm := &fakeViewManager{initiateViewErr: errors.New("initiate_view_error")}
		tp := &fakeTracerProvider{}
		vh := &viewHandler{viewManager: vm, tracer: tp.Tracer("view_handler")}

		_, err := vh.callView(context.Background(), command)
		require.Error(t, err)
		require.Contains(t, err.Error(), "initiate_view_error")
	})
}

func TestStreamCallView(t *testing.T) {
	t.Parallel()
	vm := &fakeViewManager{}
	tp := &fakeTracerProvider{}
	marshaller := &fakeMarshaller{}

	vh := &viewHandler{
		viewManager: vm,
		tracer:      tp.Tracer("view_handler"),
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
	require.Equal(t, 1, len(mockCtx.services))
	require.Equal(t, 1, marshaller.marshalCommandResponseCallCount)
	require.Equal(t, 1, len(scs.sentMsgs))
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
		tp := &fakeTracerProvider{}
		vh := &viewHandler{viewManager: vm, tracer: tp.Tracer("view_handler")}
		marshaller := &fakeMarshaller{}

		err := vh.streamCallView(sc, command, &viewTestStreamServer{}, marshaller)
		require.Error(t, err)
		require.Contains(t, err.Error(), "new_view_error")
	})

	t.Run("InitiateContext fails", func(t *testing.T) {
		t.Parallel()
		vm := &fakeViewManager{initiateContextErr: errors.New("initiate_context_error")}
		tp := &fakeTracerProvider{}
		vh := &viewHandler{viewManager: vm, tracer: tp.Tracer("view_handler")}
		marshaller := &fakeMarshaller{}

		err := vh.streamCallView(sc, command, &viewTestStreamServer{}, marshaller)
		require.Error(t, err)
		require.Contains(t, err.Error(), "initiate_context_error")
	})

	t.Run("RunView fails", func(t *testing.T) {
		t.Parallel()
		mockCtx := &fakeContext{id: "ctx", runViewErr: errors.New("run_view_error")}
		vm := &fakeViewManager{initiateContextReturns: mockCtx}
		tp := &fakeTracerProvider{}
		vh := &viewHandler{viewManager: vm, tracer: tp.Tracer("view_handler")}
		marshaller := &fakeMarshaller{}

		err := vh.streamCallView(sc, command, &viewTestStreamServer{}, marshaller)
		require.Error(t, err)
		require.Contains(t, err.Error(), "run_view_error")
	})
}

func TestCallView_JSON(t *testing.T) {
	t.Parallel()
	tp := &fakeTracerProvider{}
	command := &protos.Command{
		Payload: &protos.Command_CallView{
			CallView: &protos.CallView{Fid: "test_view", Input: []byte("test_input")},
		},
	}

	t.Run("non-byte result is JSON marshalled", func(t *testing.T) {
		t.Parallel()
		vm := &fakeViewManager{initiateViewReturns: map[string]string{"key": "value"}}
		vh := &viewHandler{viewManager: vm, tracer: tp.Tracer("view_handler")}

		result, err := vh.callView(context.Background(), command)
		require.NoError(t, err)
		resp, ok := result.(*protos.CommandResponse_CallViewResponse)
		require.True(t, ok)
		require.Contains(t, string(resp.CallViewResponse.Result), "value")
	})

	t.Run("unmarshalable result returns error", func(t *testing.T) {
		t.Parallel()
		vm := &fakeViewManager{initiateViewReturns: make(chan int)}
		vh := &viewHandler{viewManager: vm, tracer: tp.Tracer("view_handler")}

		_, err := vh.callView(context.Background(), command)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed marshalling result")
	})
}

func TestStreamCallView_JSON(t *testing.T) {
	t.Parallel()
	tp := &fakeTracerProvider{}
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
		vh := &viewHandler{viewManager: vm, tracer: tp.Tracer("view_handler")}
		marshaller := &fakeMarshaller{marshalCommandResponseReturns: &protos.SignedCommandResponse{}}

		err := vh.streamCallView(sc, command, &viewTestStreamServer{}, marshaller)
		require.NoError(t, err)
	})

	t.Run("unmarshalable result returns error", func(t *testing.T) {
		t.Parallel()
		mockCtx := &fakeContext{id: "ctx", runViewReturns: make(chan int)}
		vm := &fakeViewManager{initiateContextReturns: mockCtx}
		vh := &viewHandler{viewManager: vm, tracer: tp.Tracer("view_handler")}
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
