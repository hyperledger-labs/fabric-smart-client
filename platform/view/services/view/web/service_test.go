/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/wsproto"
)

// echoService answers every command with a response naming the command's hash, records the
// TLS state it saw, and streams back what the client sends until it sends "done".
type echoService struct {
	protos.UnimplementedViewServiceServer
	sawTLS     bool
	processErr error
	streamErr  error
}

// respond builds the response the view service would send: binary CommandResponse bytes
// inside a SignedCommandResponse, as on gRPC.
func respond(sc *protos.SignedCommand, result string) *protos.SignedCommandResponse {
	h := sha256.Sum256(sc.Command)
	raw, _ := proto.Marshal(&protos.CommandResponse{
		Header:  &protos.CommandResponseHeader{CommandHash: h[:]},
		Payload: &protos.CommandResponse_CallViewResponse{CallViewResponse: &protos.CallViewResponse{Result: []byte(result)}},
	})
	return &protos.SignedCommandResponse{Response: raw}
}

func (s *echoService) ProcessCommand(ctx context.Context, sc *protos.SignedCommand) (*protos.SignedCommandResponse, error) {
	if p, ok := peer.FromContext(ctx); ok {
		_, s.sawTLS = p.AuthInfo.(credentials.TLSInfo)
	}
	if s.processErr != nil {
		return nil, s.processErr
	}
	return respond(sc, "ok"), nil
}

func (s *echoService) StreamCommand(stream protos.ViewService_StreamCommandServer) error {
	sc, err := stream.Recv()
	if err != nil {
		return err
	}
	for {
		m := &protos.CallViewResponse{}
		if err := stream.RecvMsg(m); err != nil {
			return err
		}
		if string(m.Result) == "done" {
			break
		}
		if err := stream.SendMsg(m); err != nil {
			return err
		}
	}
	if s.streamErr != nil {
		return s.streamErr
	}
	return stream.Send(respond(sc, "final"))
}

func newTestServer(t *testing.T, svc protos.ViewServiceServer, maxRecv int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle(protos.ProcessCommandHTTPPath, NewProcessCommandHandler(svc, maxRecv, time.Minute))
	mux.Handle(protos.StreamCommandHTTPPath, NewStreamCommandHandler(svc, maxRecv))
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func post(t *testing.T, url string, body []byte) (int, []byte) {
	t.Helper()
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, raw
}

func TestProcessCommandHandler(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, &echoService{}, 1024)
	sc := &protos.SignedCommand{Command: []byte("cmd"), Signature: []byte("sig")}
	body, err := protojson.Marshal(sc)
	require.NoError(t, err)

	code, raw := post(t, ts.URL+protos.ProcessCommandHTTPPath, body)
	require.Equal(t, http.StatusOK, code)
	got := &protos.SignedCommandResponse{}
	require.NoError(t, protojson.Unmarshal(raw, got))
	require.Equal(t, respond(sc, "ok").Response, got.Response)

	code, _ = post(t, ts.URL+protos.ProcessCommandHTTPPath, []byte("not json"))
	require.Equal(t, http.StatusBadRequest, code)

	code, _ = post(t, ts.URL+protos.ProcessCommandHTTPPath, bytes.Repeat([]byte("a"), 2048))
	require.Equal(t, http.StatusRequestEntityTooLarge, code)

	resp, err := http.Get(ts.URL + protos.ProcessCommandHTTPPath)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

func TestProcessCommandHandlerServiceError(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, &echoService{processErr: errors.New("boom")}, 1024)
	body, _ := protojson.Marshal(&protos.SignedCommand{Command: []byte("cmd")})
	code, raw := post(t, ts.URL+protos.ProcessCommandHTTPPath, body)
	require.Equal(t, http.StatusInternalServerError, code)
	require.NotContains(t, string(raw), "boom", "internal error text stays on the server")
}

// The view may run longer than the listener's write timeout; the handler extends the write
// deadline to the view timeout, so the response still arrives.
func TestProcessCommandHandlerOutlivesWriteTimeout(t *testing.T) {
	t.Parallel()
	slow := &slowService{delay: 300 * time.Millisecond}
	ts := httptest.NewUnstartedServer(NewProcessCommandHandler(slow, 1024, time.Minute))
	ts.Config.WriteTimeout = 100 * time.Millisecond
	ts.Start()
	t.Cleanup(ts.Close)
	body, _ := protojson.Marshal(&protos.SignedCommand{Command: []byte("cmd")})
	code, _ := post(t, ts.URL, body)
	require.Equal(t, http.StatusOK, code)
}

type slowService struct {
	protos.UnimplementedViewServiceServer
	delay time.Duration
}

func (s *slowService) ProcessCommand(_ context.Context, sc *protos.SignedCommand) (*protos.SignedCommandResponse, error) {
	time.Sleep(s.delay)
	return respond(sc, "ok"), nil
}

func dial(t *testing.T, ts *httptest.Server) *websocket.Conn {
	t.Helper()
	conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(ts.URL, "http")+protos.StreamCommandHTTPPath, nil)
	require.NoError(t, err)
	_ = resp.Body.Close()
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestStreamCommandHandler(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, &echoService{}, 1024)
	conn := dial(t, ts)
	sc := &protos.SignedCommand{Command: []byte("cmd")}
	require.NoError(t, wsproto.Write(conn, sc))
	require.NoError(t, wsproto.Write(conn, &protos.CallViewResponse{Result: []byte("hello")}))
	echo := &protos.CallViewResponse{}
	require.NoError(t, wsproto.Read(conn, echo))
	require.Equal(t, "hello", string(echo.Result))
	require.NoError(t, wsproto.Write(conn, &protos.CallViewResponse{Result: []byte("done")}))
	final := &protos.SignedCommandResponse{}
	require.NoError(t, wsproto.Read(conn, final))
	require.Equal(t, respond(sc, "final").Response, final.Response)
	_, _, err := conn.ReadMessage()
	require.True(t, websocket.IsCloseError(err, websocket.CloseNormalClosure), "got %v", err)
}

func TestStreamCommandHandlerError(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, &echoService{streamErr: errors.New("boom")}, 1024)
	conn := dial(t, ts)
	require.NoError(t, wsproto.Write(conn, &protos.SignedCommand{Command: []byte("cmd")}))
	require.NoError(t, wsproto.Write(conn, &protos.CallViewResponse{Result: []byte("done")}))
	_, _, err := conn.ReadMessage()
	require.True(t, websocket.IsCloseError(err, websocket.CloseInternalServerErr), "got %v", err)
}

func TestStreamCommandHandlerReadLimit(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, &echoService{}, 64)
	conn := dial(t, ts)
	require.NoError(t, wsproto.Write(conn, &protos.SignedCommand{Command: bytes.Repeat([]byte("a"), 256)}))
	_, _, err := conn.ReadMessage()
	require.Error(t, err, "an oversized frame ends the stream")
}

func TestWithPeerCarriesTLS(t *testing.T) {
	t.Parallel()
	svc := &echoService{}
	ts := httptest.NewTLSServer(NewProcessCommandHandler(svc, 1024, time.Minute))
	t.Cleanup(ts.Close)
	body, _ := protojson.Marshal(&protos.SignedCommand{Command: []byte("cmd")})
	resp, err := ts.Client().Post(ts.URL, "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.True(t, svc.sawTLS)
}

// blockingStreamService waits for a client message that never comes.
type blockingStreamService struct {
	protos.UnimplementedViewServiceServer
	entered chan struct{}
}

func (s *blockingStreamService) StreamCommand(stream protos.ViewService_StreamCommandServer) error {
	if _, err := stream.Recv(); err != nil {
		return err
	}
	close(s.entered)
	return stream.RecvMsg(&protos.CallViewResponse{})
}

// Cancelling the request context, as the web listener's Stop does, ends a stream whose view
// is blocked waiting for the client.
func TestStreamCommandHandlerStopsOnCancel(t *testing.T) {
	t.Parallel()
	svc := &blockingStreamService{entered: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	returned := make(chan struct{})
	h := NewStreamCommandHandler(svc, 1024)
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(returned)
		h.ServeHTTP(w, r.WithContext(ctx))
	}))
	ts.Start()
	t.Cleanup(ts.Close)

	conn := dial(t, ts)
	require.NoError(t, wsproto.Write(conn, &protos.SignedCommand{Command: []byte("cmd")}))
	<-svc.entered
	cancel()
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("stream handler did not return after its context was cancelled")
	}
}
