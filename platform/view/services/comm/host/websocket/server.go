/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
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

func newHandler(streamProvider serverStreamProvider, newStreamCallback func(stream host2.P2PStream), corsAllowedOrigins []string) *gin.Engine {
	logger.Debugf("creating GIN engine for p2p REST endpoint.")
	r := gin.New()

	// Recovery middleware to catch panics and log them
	r.Use(gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		logger.Errorf("Panic recovered in websocket handler from [%s]: %v", c.Request.RemoteAddr, recovered)
		c.AbortWithStatus(http.StatusInternalServerError)
	}))

	r.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		// Log errors at ERROR level, others at INFO level
		if param.StatusCode >= 400 {
			logger.Errorf("HTTP %d - %s %s from [%s]: %s",
				param.StatusCode, param.Method, param.Path, param.ClientIP, param.ErrorMessage)
		}
		// Standard format for access logs
		return fmt.Sprintf("%s - [%s] \"%s %s %s %d %s \"%s\" %s\"\n",
			param.ClientIP,
			param.TimeStamp.Format(time.RFC1123),
			param.Method,
			param.Path,
			param.Request.Proto,
			param.StatusCode,
			param.Latency,
			param.Request.UserAgent(),
			param.ErrorMessage,
		)
	}))
	if len(corsAllowedOrigins) > 0 {
		config := cors.DefaultConfig()
		config.AllowOrigins = corsAllowedOrigins
		config.AllowHeaders = []string{"Origin", "Content-Type", "Accept"}
		r.Use(cors.New(config))
	}

	r.GET("/p2p", func(c *gin.Context) {
		logger.DebugfContext(c.Request.Context(), "new incoming stream from [%s]", c.Request.RemoteAddr)
		if err := streamProvider.NewServerStream(c.Writer, c.Request, newStreamCallback); err != nil {
			logger.ErrorfContext(c.Request.Context(), "error receiving websocket: %s", err.Error())
		}
	})
	return r
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
