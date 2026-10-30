/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package web

import (
	"context"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/wsproto"
)

const (
	// closeTimeout bounds writing the close frame.
	closeTimeout = time.Second
	// drainTimeout bounds discarding unread input while waiting for the peer's close frame.
	drainTimeout = 100 * time.Millisecond
)

// NewStreamCommandHandler serves StreamCommand over a WebSocket. svc sees the connection as a
// protos.ViewService_StreamCommandServer whose messages are wsproto frames. When svc returns,
// the connection closes normally, or when svc returned an error with code 1013 (try again
// later) for a node that drains before it stops, and with code 1011 otherwise, the reason
// naming a view the node's shutdown cancelled; svc reports command failures in its final
// response before that, as on gRPC. When the request context
// ends, the connection is closed, so a view waiting for the client returns.
func NewStreamCommandHandler(svc protos.ViewServiceServer, maxRecvMsgSize int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := server.OpenWSServerConnWithLimit(w, r, int64(maxRecvMsgSize))
		if err != nil {
			return // the upgrader has answered the request
		}
		stream := &wsCommandStream{conn: conn, ctx: withPeer(r.Context(), r)}
		// A hijacked connection outlives its request context, and a view may be blocked
		// reading from the client. Closing the connection when the context ends, as the web
		// listener's Stop does, unblocks it.
		stop := context.AfterFunc(r.Context(), func() { _ = conn.Close() })
		defer stop()
		if err := svc.StreamCommand(stream); err != nil {
			switch st := status.Convert(err); st.Code() {
			case codes.Unavailable:
				stream.close(websocket.CloseTryAgainLater, st.Message())
			case codes.Aborted:
				// No close code means "check the outcome"; the reason carries it.
				stream.close(websocket.CloseInternalServerErr, st.Message())
			default:
				logger.Errorf("view service stream from [%s] failed: %s", r.RemoteAddr, err)
				stream.close(websocket.CloseInternalServerErr, "stream command failed")
			}
			return
		}
		stream.close(websocket.CloseNormalClosure, "")
	})
}

// wsCommandStream is a protos.ViewService_StreamCommandServer over a WebSocket. Headers and
// trailers have no WebSocket equivalent and are dropped.
type wsCommandStream struct {
	conn *websocket.Conn
	ctx  context.Context
}

func (s *wsCommandStream) Send(m *protos.SignedCommandResponse) error {
	return wsproto.Write(s.conn, m)
}

func (s *wsCommandStream) Recv() (*protos.SignedCommand, error) {
	m := &protos.SignedCommand{}
	if err := wsproto.Read(s.conn, m); err != nil {
		return nil, err
	}
	return m, nil
}

func (s *wsCommandStream) SendMsg(m any) error {
	pm, ok := m.(proto.Message)
	if !ok {
		return errors.Errorf("cannot send [%T]: not a protobuf message", m)
	}
	return wsproto.Write(s.conn, pm)
}

func (s *wsCommandStream) RecvMsg(m any) error {
	pm, ok := m.(proto.Message)
	if !ok {
		return errors.Errorf("cannot receive into [%T]: not a protobuf message", m)
	}
	return wsproto.Read(s.conn, pm)
}

func (*wsCommandStream) SetHeader(metadata.MD) error  { return nil }
func (*wsCommandStream) SendHeader(metadata.MD) error { return nil }
func (*wsCommandStream) SetTrailer(metadata.MD)       {}
func (s *wsCommandStream) Context() context.Context   { return s.ctx }

// close sends a close frame and waits briefly for the peer's, discarding unread input:
// closing a socket that still holds unread data resets the connection, and the peer could
// lose the final response.
func (s *wsCommandStream) close(code int, reason string) {
	_ = s.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, reason), time.Now().Add(closeTimeout))
	_ = s.conn.SetReadDeadline(time.Now().Add(drainTimeout))
	for {
		if _, _, err := s.conn.NextReader(); err != nil {
			break
		}
	}
	_ = s.conn.Close()
}
