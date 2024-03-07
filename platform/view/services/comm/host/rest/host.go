/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("rest-p2p-host")

type host struct {
	listenAddress     host2.PeerIPAddress
	certFile, keyFile string

	nodeID  host2.PeerID
	routing routing
	server  io.Closer
}

func NewHost(nodeID host2.PeerID, listenAddress host2.PeerIPAddress, routing routing, certFile, keyFile string) *host {
	logger.Infof("Creating new host on %s", listenAddress)
	return &host{
		listenAddress: listenAddress,
		nodeID:        nodeID,
		routing:       routing,
		certFile:      certFile,
		keyFile:       keyFile,
	}
}

func (h *host) Start(newStreamCallback func(stream host2.P2PStream)) error {
	logger.Infof("Starting up REST server on [%s]...", h.listenAddress)
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
		logger.Infof("New incoming stream from [%s]", c.Request.RemoteAddr)
		stream, err := newServerStream(c.Writer, c.Request)
		if err != nil {
			logger.Errorf("error receiving websocket: %v", err)
		}
		newStreamCallback(stream)
	})

	go func() {
		if err := r.Run(h.listenAddress); err != nil {
			panic(err)
		}
	}()
	return nil
}

func (h *host) NewStream(_ context.Context, address host2.PeerIPAddress, peerID host2.PeerID) (host2.P2PStream, error) {
	if len(address) == 0 {
		logger.Debugf("No address passed for peer [%s]. Resolving...", peerID)
		addresses, ok := h.routing.Lookup(peerID)
		if !ok || len(addresses) == 0 {
			return nil, errors.Errorf("no address found for peer [%s]", peerID)
		}
		logger.Debugf("Resolved %d addresses of peer [%s] and picking the first one.", len(addresses), peerID)
		address = addresses[0]
	}
	return newClientStream(address, h.nodeID, peerID)
}

func (h *host) Lookup(peerID host2.PeerID) ([]host2.PeerIPAddress, bool) {
	return h.routing.Lookup(peerID)
}

func (h *host) Close() error {
	logger.Infof("Shutting down REST server")
	if h.server != nil {
		return h.server.Close()
	}
	return nil
}

func (h *host) Wait() {}
