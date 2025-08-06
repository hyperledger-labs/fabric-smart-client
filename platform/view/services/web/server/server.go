/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	middleware2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server/middleware"
)

var logger = logging.MustGetLogger()

//go:generate counterfeiter -o fakes/logger.go -fake-name Logger . Logger

type TLS struct {
	Enabled           bool
	CertFile          string
	KeyFile           string
	ClientAuth        bool
	ClientCACertFiles []string
}

func (t TLS) Config() (*tls.Config, error) {
	var tlsConfig *tls.Config

	if !t.Enabled {
		return tlsConfig, nil
	}
	cert, err := tls.LoadX509KeyPair(t.CertFile, t.KeyFile)
	if err != nil {
		return nil, err
	}

	if len(t.ClientCACertFiles) == 0 {
		return nil, errors.Errorf("client TLS CA certificate pool must not be empty")
	}

	caCertPool := x509.NewCertPool()
	for _, caPath := range t.ClientCACertFiles {
		caPem, err := os.ReadFile(caPath)
		if err != nil {
			return nil, err
		}
		caCertPool.AppendCertsFromPEM(caPem)
	}
	tlsConfig = &tls.Config{
		Certificates: []tls.Certificate{cert},
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		},
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,
		ClientCAs:  caCertPool,
	}
	if t.ClientAuth {
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	} else {
		tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
	}
	return tlsConfig, nil
}

type Options struct {
	ListenAddress string
	TLS           TLS
}

type Server struct {
	options    Options
	httpServer *http.Server
	mux        *http.ServeMux
	addr       string
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

	go utils.IgnoreErrorFunc(func() error {
		return s.httpServer.Serve(listener)
	})

	return nil
}

func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return s.httpServer.Shutdown(ctx)
}

func (s *Server) initializeServer() {
	s.mux = http.NewServeMux()
	s.httpServer = &http.Server{
		Addr:         s.options.ListenAddress,
		Handler:      s.mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 2 * time.Minute,
	}
}

func (s *Server) HandlerChain(h http.Handler, secure bool) http.Handler {
	if secure {
		return middleware2.NewChain(middleware2.RequireCert(), middleware2.WithRequestID(GenerateUUID)).Handler(h)
	}
	return middleware2.NewChain(middleware2.WithRequestID(GenerateUUID)).Handler(h)
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
	tlsConfig, err := s.options.TLS.Config()
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
