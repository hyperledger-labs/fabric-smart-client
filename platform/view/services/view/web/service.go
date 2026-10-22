/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package web is the HTTP binding of the view service (see protos.ProcessCommandHTTPPath).
// Its handlers decode protojson messages and call the same protos.ViewServiceServer that
// serves gRPC, so authentication, binding, replay protection, view execution and response
// signing are one implementation for both transports.
package web

import (
	"context"
	stderrors "errors"
	"io"
	"net"
	"net/http"
	"time"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
)

var logger = logging.MustGetLogger()

// writeSlack is how long a response may take to write once the view's deadline has passed.
const writeSlack = 30 * time.Second

// NewProcessCommandHandler serves ProcessCommand. Every request whose body decodes as a
// SignedCommand gets 200 and the SignedCommandResponse svc returns, failures included, as on
// gRPC. Other statuses report what keeps a command from existing: 400 for an undecodable
// body, 405 for a method other than POST, 413 for a body over maxRecvMsgSize, and 500 when
// svc itself returns an error. The write deadline is extended to viewTimeout plus slack, so
// a view that runs up to its timeout still delivers its response.
func NewProcessCommandHandler(svc protos.ViewServiceServer, maxRecvMsgSize int, viewTimeout time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(viewTimeout + writeSlack))

		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, int64(maxRecvMsgSize)))
		if err != nil {
			if _, ok := stderrors.AsType[*http.MaxBytesError](err); ok {
				http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, "failed reading request body", http.StatusBadRequest)
			return
		}
		sc := &protos.SignedCommand{}
		if err := protojson.Unmarshal(body, sc); err != nil {
			http.Error(w, "request body is not a SignedCommand", http.StatusBadRequest)
			return
		}
		resp, err := svc.ProcessCommand(withPeer(r.Context(), r), sc)
		if err != nil {
			logger.Errorf("view service failed processing command from [%s]: %s", r.RemoteAddr, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		raw, err := protojson.Marshal(resp)
		if err != nil {
			logger.Errorf("failed encoding view service response: %s", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	})
}

// withPeer returns ctx carrying r's caller address and TLS state as a gRPC peer, the form
// the view service reads them in on every transport.
func withPeer(ctx context.Context, r *http.Request) context.Context {
	p := &peer.Peer{Addr: remoteAddr(r.RemoteAddr)}
	if r.TLS != nil {
		p.AuthInfo = credentials.TLSInfo{State: *r.TLS}
	}
	return peer.NewContext(ctx, p)
}

// remoteAddr is an HTTP request's remote address as a net.Addr.
type remoteAddr string

func (remoteAddr) Network() string  { return "tcp" }
func (a remoteAddr) String() string { return string(a) }

var _ net.Addr = remoteAddr("")
