/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/pkg/errors"
)

type serverStreamProvider interface {
	NewServerStream(writer http.ResponseWriter, request *http.Request, newStreamCallback func(host2.P2PStream)) error
}

type server struct {
	srv            *http.Server
	streamProvider serverStreamProvider
}

func newHandler(streamProvider serverStreamProvider, newStreamCallback func(stream host2.P2PStream)) *gin.Engine {
	logger.Debugf("Creating GIN engine for p2p REST endpoint.")
	r := gin.New()
	r.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		// your custom format
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
	config := cors.DefaultConfig()
	config.AllowOrigins = []string{"*"}
	config.AllowHeaders = []string{"*"}
	r.Use(cors.New(config))

	r.GET("/p2p", func(c *gin.Context) {
		logger.Debugf("New incoming stream from [%s]", c.Request.RemoteAddr)
		if err := streamProvider.NewServerStream(c.Writer, c.Request, newStreamCallback); err != nil {
			logger.Errorf("error receiving websocket: %s", err.Error())
		}
	})
	return r
}

func (s *server) Start(newStreamCallback func(stream host2.P2PStream)) error {
	s.srv.Handler = newHandler(s.streamProvider, newStreamCallback)

	var err error
	if s.srv.TLSConfig == nil {
		logger.Infof("Starting up REST server without TLS on [%s]...", s.srv.Addr)
		err = s.srv.ListenAndServe()
	} else {
		logger.Infof("Starting up REST server with TLS [%v] on [%s]...", s.srv.TLSConfig, s.srv.Addr)
		err = s.srv.ListenAndServeTLS("", "")
	}
	if !errors.Is(err, http.ErrServerClosed) {
		return errors.Wrapf(err, "failed to start http server")
	}
	return nil
}

func (s *server) Close() error {
	logger.Debugf("Shutting down server on [%s]", s.srv.Addr)
	shutdownCtx, shutdownRelease := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownRelease()
	return s.srv.Shutdown(shutdownCtx)
}
