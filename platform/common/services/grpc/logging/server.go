/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package logging provides gRPC server interceptors that log each call.
package logging

import (
	"context"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
)

// UnaryServerInterceptor logs one line at Debug when a unary call completes, with the
// fields of the call (see [StreamServerInterceptor]).
func UnaryServerInterceptor(logger logging.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if !logger.IsEnabledFor(logging.DebugLevel) {
			return handler(ctx, req)
		}
		start := time.Now()
		resp, err := handler(ctx, req)
		logger.Debugw("unary call completed", callFields(ctx, info.FullMethod, start, err)...)
		return resp, err
	}
}

// StreamServerInterceptor logs one line at Debug when a streaming call completes. The
// line has the fields grpc.service, grpc.method, grpc.code and grpc.call_duration; if
// known, grpc.request_deadline, grpc.peer_address and the TLS client certificate's
// grpc.peer_subject; and, if the call failed, error.
func StreamServerInterceptor(logger logging.Logger) grpc.StreamServerInterceptor {
	return func(service any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if !logger.IsEnabledFor(logging.DebugLevel) {
			return handler(service, stream)
		}
		start := time.Now()
		err := handler(service, stream)
		logger.Debugw("streaming call completed", callFields(stream.Context(), info.FullMethod, start, err)...)
		return err
	}
}

// callFields returns the key/value pairs of a completed call.
func callFields(ctx context.Context, fullMethod string, start time.Time, err error) []any {
	fields := make([]any, 0, 16)
	if parts := strings.Split(fullMethod, "/"); len(parts) == 3 {
		fields = append(fields, "grpc.service", parts[1], "grpc.method", parts[2])
	}
	if deadline, ok := ctx.Deadline(); ok {
		fields = append(fields, "grpc.request_deadline", deadline)
	}
	if p, ok := peer.FromContext(ctx); ok {
		fields = append(fields, "grpc.peer_address", p.Addr.String())
		if ti, ok := p.AuthInfo.(credentials.TLSInfo); ok && len(ti.State.PeerCertificates) > 0 {
			fields = append(fields, "grpc.peer_subject", ti.State.PeerCertificates[0].Subject.String())
		}
	}
	fields = append(fields,
		"grpc.code", status.Code(err).String(),
		"grpc.call_duration", time.Since(start),
	)
	if err != nil {
		// err.Error(), not err: zap would add a stack trace for an error that
		// implements fmt.Formatter.
		fields = append(fields, "error", err.Error())
	}
	return fields
}
