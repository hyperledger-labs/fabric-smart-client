/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
)

type serverStreamProvider interface {
	NewServerStream(writer http.ResponseWriter, request *http.Request, newStreamCallback func(host2.P2PStream)) error
}

type server struct {
	srv                *http.Server
	streamProvider     serverStreamProvider
	listener           net.Listener
	corsAllowedOrigins []string
}

func (s *server) Listen() error {
	l, err := net.Listen("tcp", s.srv.Addr)
	if err != nil {
		logger.Errorf("Failed to listen on address [%s]: %s", s.srv.Addr, err.Error())
		return errors.Wrapf(err, "failed to listen on [%s]", s.srv.Addr)
	}
	s.srv.Addr = l.Addr().String()
	s.listener = l
	return nil
}

func (s *server) Start(newStreamCallback func(stream host2.P2PStream)) error {
	s.srv.Handler = newHandler(s.streamProvider, newStreamCallback, s.corsAllowedOrigins)

	var err error
	if s.srv.TLSConfig == nil {
		logger.Infof("starting up REST server without TLS on [%s]...", s.srv.Addr)
		err = s.srv.Serve(s.listener)
	} else {
		// mTLS Enforcement: s.srv.TLSConfig contains the configuration to:
		// 1. Require client certificates (tls.RequireAndVerifyClientCert).
		// 2. Verify them against the root CAs.
		// The underlying ListenAndServeTLS call uses this config to secure the connection.
		logger.Infof("starting up REST server with TLS on [%s]...", s.srv.Addr)
		err = s.srv.ServeTLS(s.listener, "", "")
	}

	if !errors.Is(err, http.ErrServerClosed) {
		logger.Errorf("HTTP server failed on [%s]: %s", s.srv.Addr, err.Error())
		return errors.Wrapf(err, "failed to start http server")
	}
	return nil
}

func (s *server) Close() error {
	logger.Debugf("shutting down server on [%s]", s.srv.Addr)
	shutdownCtx, shutdownRelease := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownRelease()
	return s.srv.Shutdown(shutdownCtx)
}

func newHandler(streamProvider serverStreamProvider, newStreamCallback func(stream host2.P2PStream), corsAllowedOrigins []string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /p2p", func(w http.ResponseWriter, r *http.Request) {
		logger.DebugfContext(r.Context(), "new incoming stream from [%s]", r.RemoteAddr)
		if err := streamProvider.NewServerStream(w, r, newStreamCallback); err != nil {
			logger.ErrorfContext(r.Context(), "error receiving websocket: %s", err.Error())
		}
	})

	var h http.Handler = mux
	if len(corsAllowedOrigins) > 0 {
		h = withCORS(corsAllowedOrigins, h)
	}
	h = withLogging(h)
	h = withRecovery(h)
	return h
}

func withRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Errorf("Panic recovered in websocket handler from [%s]: %v", r.RemoteAddr, rec)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		logger.Infof("%s - \"%s %s %s\" %s", r.RemoteAddr, r.Method, r.URL.Path, r.Proto, time.Since(start))
	})
}

func withCORS(allowedOrigins []string, next http.Handler) http.Handler {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[o] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && allowed[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Accept")
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
