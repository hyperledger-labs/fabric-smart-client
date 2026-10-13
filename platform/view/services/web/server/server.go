/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	utils2 "github.com/hyperledger-labs/fabric-smart-client/pkg/utils"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	middleware2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server/middleware"
)

var logger = logging.MustGetLogger()

var (
	// shutdownGracePeriod bounds how long Stop lets in-flight handlers run undisturbed.
	shutdownGracePeriod = 5 * time.Second
	// handlerWaitTimeout bounds how long Stop waits for handlers once their contexts are cancelled.
	handlerWaitTimeout = 10 * time.Second
)

type Options struct {
	ListenAddress string
	// TLS is the resolved server-side TLS for this listener. Resolution, validation and
	// file loading all happen in platform/common/services/tlsconfig before it gets here.
	TLS grpc.SecureOptions
}

// Server is an HTTP(S) listener serving the handlers registered on it.
type Server struct {
	options    Options
	httpServer *http.Server
	mux        *http.ServeMux
	addr       string

	// cancelRequests cancels the base context every request context derives from.
	cancelRequests context.CancelFunc
	// mu guards stopping and orders handlers.Add before handlers.Wait.
	mu       sync.Mutex
	stopping bool
	// handlers counts running handlers, including those that hijacked their connection
	// (streaming views), which http.Server.Shutdown neither waits for nor closes. A handler
	// that hands its connection to another goroutine and returns is no longer counted.
	handlers sync.WaitGroup
}

func NewServer(o Options) *Server {
	server := &Server{
		options: o,
	}

	server.initializeServer()

	return server
}

func (s *Server) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	err := s.Start()
	if err != nil {
		return err
	}

	close(ready)

	<-signals
	return s.Stop()
}

func (s *Server) Start() error {
	listener, err := s.listen()
	if err != nil {
		return err
	}
	s.addr = listener.Addr().String()

	go func() {
		_ = s.httpServer.Serve(listener)
	}()

	return nil
}

// Stop closes the listener and returns once every in-flight handler has returned, so
// callers can release resources the handlers use. Handlers first get a grace period of 5s
// to finish; the contexts of those still running are then cancelled, and Stop waits up to
// 10s more. It returns an error when handlers are still running after that; they are left
// running.
func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownGracePeriod)
	defer cancel()
	// Shutdown's error only reports connections still active at the deadline; the
	// handler wait below covers those and the hijacked ones it does not track.
	_ = s.httpServer.Shutdown(ctx)

	s.mu.Lock()
	s.stopping = true
	s.mu.Unlock()
	done := make(chan struct{})
	go func() {
		s.handlers.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
	}
	logger.Warnf("in-flight handlers still running after %s, cancelling their contexts", shutdownGracePeriod)
	s.cancelRequests()
	select {
	case <-done:
		return nil
	case <-time.After(handlerWaitTimeout):
		return errors.Errorf("in-flight handlers did not return within %s of cancelling their contexts", handlerWaitTimeout)
	}
}

// track counts the handlers h runs, so Stop can wait for them, and rejects requests
// that arrive once Stop has started waiting.
func (s *Server) track(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		if s.stopping {
			s.mu.Unlock()
			http.Error(w, "server is shutting down", http.StatusServiceUnavailable)
			return
		}
		s.handlers.Add(1)
		s.mu.Unlock()
		defer s.handlers.Done()
		h.ServeHTTP(w, r)
	})
}

func (s *Server) initializeServer() {
	baseCtx, cancel := context.WithCancel(context.Background())
	s.cancelRequests = cancel
	s.mux = http.NewServeMux()
	s.httpServer = &http.Server{
		Addr:              s.options.ListenAddress,
		Handler:           s.track(s.mux),
		BaseContext:       func(net.Listener) context.Context { return baseCtx },
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}
}

func (*Server) HandlerChain(h http.Handler, secure bool) http.Handler {
	if secure {
		return middleware2.NewChain(middleware2.RequireCert(), middleware2.WithRequestID(utils2.GenerateUUID)).Handler(h)
	}
	return middleware2.NewChain(middleware2.WithRequestID(utils2.GenerateUUID)).Handler(h)
}

// RegisterHandler registers into the ServeMux a handler chain that borrows
// its security properties from the fabhttp.Server. This method is thread
// safe because ServeMux.Handle() is thread safe, and options are immutable.
// This method can be called either before or after Server.Start(). If the
// pattern exists the method panics.
func (s *Server) RegisterHandler(pattern string, handler http.Handler, secure bool) {
	logger.Debugf("register handler [%s][%v]", pattern, secure)
	s.mux.Handle(
		pattern,
		s.HandlerChain(
			handler,
			secure,
		),
	)
}

func (s *Server) Addr() string {
	return s.addr
}

func (s *Server) listen() (net.Listener, error) {
	listener, err := net.Listen("tcp", s.options.ListenAddress)
	if err != nil {
		return nil, err
	}
	tlsConfig, err := s.options.TLS.TLSConfig()
	if err != nil {
		return nil, err
	}
	if tlsConfig != nil {
		logger.Infof("TLS enabled")
		listener = tls.NewListener(listener, tlsConfig)
	} else {
		logger.Infof("TLS disabled")
	}
	return listener, nil
}
