/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc

import (
	"context"
	"strings"
	"sync/atomic"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
)

// DisconnectTracker warns when a client disconnect
type DisconnectTracker struct {
	ClientName string
	Count      uint64
}

func (h *DisconnectTracker) TagRPC(ctx context.Context, info *stats.RPCTagInfo) context.Context {
	return ctx
}

func (h *DisconnectTracker) HandleRPC(ctx context.Context, rpcStats stats.RPCStats) {
	// Check RPC errors for the specific code
	if end, ok := rpcStats.(*stats.End); ok && end.Error != nil {
		st, _ := status.FromError(end.Error)
		// Check for ResourceExhausted + specific message
		if st.Code() == codes.ResourceExhausted &&
			(strings.Contains(st.Message(), "too_many_pings") ||
				strings.Contains(st.Message(), "ENHANCE_YOUR_CALM")) {
			atomic.AddUint64(&h.Count, 1)
			logger.Errorf("ResourceExhausted for %s [%s][%s]", h.ClientName, st.Message(), end.Error)
		}
	}
}

func (h *DisconnectTracker) TagConn(ctx context.Context, info *stats.ConnTagInfo) context.Context {
	return ctx
}

func (h *DisconnectTracker) HandleConn(ctx context.Context, s stats.ConnStats) {
	// This fires when a connection ends
	if _, ok := s.(*stats.ConnEnd); ok {
		// You won't get the specific "error message" here easily,
		// but you will see frequent disconnects for this specific client name.
		logger.Warnf("Connection ended for client: %s", h.ClientName)
	}
}

func ErrorParser(logger logging.Logger, label string, err error) error {
	st, ok := status.FromError(err)
	if ok {
		// Check for the specific error code and message
		if st.Code() == codes.ResourceExhausted &&
			(strings.Contains(st.Message(), "too_many_pings") ||
				strings.Contains(st.Message(), "ENHANCE_YOUR_CALM")) {

			logger.Warnf("🚨 Client [%s] received ENHANCE_YOUR_CALM! Fix keepalive config.", label)
		}
	}
	return err
}
