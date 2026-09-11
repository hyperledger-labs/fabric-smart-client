/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// observedLogger returns a logger that records everything written to it,
// including payload entries, which sit below debug.
func observedLogger() (*zap.Logger, *observer.ObservedLogs) {
	core, logs := observer.New(DefaultPayloadLevel)

	return zap.New(core), logs
}

// fakeServerStream is a grpc.ServerStream that records what passed through it.
type fakeServerStream struct {
	ctx      context.Context
	sent     []any
	recvErr  error
	sendErr  error
	recvFill func(msg any)
}

func (*fakeServerStream) SetHeader(metadata.MD) error  { return nil }
func (*fakeServerStream) SendHeader(metadata.MD) error { return nil }
func (*fakeServerStream) SetTrailer(metadata.MD)       {}

func (f *fakeServerStream) Context() context.Context {
	if f.ctx != nil {
		return f.ctx
	}

	return context.Background()
}

func (f *fakeServerStream) SendMsg(m any) error {
	f.sent = append(f.sent, m)

	return f.sendErr
}

func (f *fakeServerStream) RecvMsg(m any) error {
	if f.recvFill != nil {
		f.recvFill(m)
	}

	return f.recvErr
}

// TestLevelerFunc checks the adapter forwards to the wrapped function for both
// the level and payload-level methods.
func TestLevelerFunc(t *testing.T) {
	t.Parallel()

	var gotMethod string
	f := LevelerFunc(func(_ context.Context, fullMethod string) zapcore.Level {
		gotMethod = fullMethod

		return zapcore.WarnLevel
	})

	require.Equal(t, zapcore.WarnLevel, f.Level(t.Context(), "/svc/Method"))
	require.Equal(t, "/svc/Method", gotMethod)

	require.Equal(t, zapcore.WarnLevel, f.PayloadLevel(t.Context(), "/other/Method"))
	require.Equal(t, "/other/Method", gotMethod)
}

// TestApplyOptionsDefaults checks the levels used when no options are given.
func TestApplyOptionsDefaults(t *testing.T) {
	t.Parallel()

	o := applyOptions()

	require.Equal(t, zapcore.DebugLevel, o.Level(t.Context(), "/svc/Method"))
	require.Equal(t, DefaultPayloadLevel, o.PayloadLevel(t.Context(), "/svc/Method"))
}

// TestApplyOptionsOverrides checks each option replaces only its own leveler.
func TestApplyOptionsOverrides(t *testing.T) {
	t.Parallel()

	o := applyOptions(
		WithLeveler(LevelerFunc(func(context.Context, string) zapcore.Level { return zapcore.ErrorLevel })),
	)
	require.Equal(t, zapcore.ErrorLevel, o.Level(t.Context(), "/svc/Method"))
	require.Equal(t, DefaultPayloadLevel, o.PayloadLevel(t.Context(), "/svc/Method"), "payload leveler keeps its default")

	o = applyOptions(
		WithPayloadLeveler(LevelerFunc(func(context.Context, string) zapcore.Level { return zapcore.InfoLevel })),
	)
	require.Equal(t, zapcore.DebugLevel, o.Level(t.Context(), "/svc/Method"), "leveler keeps its default")
	require.Equal(t, zapcore.InfoLevel, o.PayloadLevel(t.Context(), "/svc/Method"))
}

// TestWithFields checks the fields round-trip through the context.
func TestWithFields(t *testing.T) {
	t.Parallel()

	fields := []zapcore.Field{zap.String("k", "v")}
	ctx := WithFields(t.Context(), fields)

	stored, ok := ctx.Value(fieldKey).([]zapcore.Field)
	require.True(t, ok)
	require.Equal(t, fields, stored)
}

// TestGetFieldsMethodParsing checks the service and method are only derived
// from a well-formed full method name.
func TestGetFieldsMethodParsing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		expect map[string]string
	}{
		{"well formed", "/the.Service/TheMethod", map[string]string{
			"grpc.service": "the.Service",
			"grpc.method":  "TheMethod",
		}},
		{"no leading slash", "the.Service.TheMethod", nil},
		{"too many parts", "/a/b/c", nil},
		{"empty", "", nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fields := fieldMap(getFields(t.Context(), tc.method))

			if tc.expect == nil {
				require.NotContains(t, fields, "grpc.service")
				require.NotContains(t, fields, "grpc.method")

				return
			}
			for k, v := range tc.expect {
				require.Equal(t, v, fields[k])
			}
		})
	}
}

// TestGetFieldsDeadline checks a context deadline is reported, and absent when
// the context has none.
func TestGetFieldsDeadline(t *testing.T) {
	t.Parallel()

	require.NotContains(t, fieldMap(getFields(t.Context(), "/svc/M")), "grpc.request_deadline")

	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	require.Contains(t, fieldMap(getFields(ctx, "/svc/M")), "grpc.request_deadline")
}

// TestGetFieldsPeer covers the peer address and, when the connection carries
// TLS with a client certificate, its subject.
func TestGetFieldsPeer(t *testing.T) {
	t.Parallel()

	addr := &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 7051}

	t.Run("address only", func(t *testing.T) {
		t.Parallel()

		ctx := peer.NewContext(t.Context(), &peer.Peer{Addr: addr})
		fields := fieldMap(getFields(ctx, "/svc/M"))

		require.Equal(t, addr.String(), fields["grpc.peer_address"])
		require.NotContains(t, fields, "grpc.peer_subject")
	})

	t.Run("tls without certificates", func(t *testing.T) {
		t.Parallel()

		ctx := peer.NewContext(t.Context(), &peer.Peer{
			Addr:     addr,
			AuthInfo: credentials.TLSInfo{State: tls.ConnectionState{}},
		})

		require.NotContains(t, fieldMap(getFields(ctx, "/svc/M")), "grpc.peer_subject")
	})

	t.Run("tls with a client certificate", func(t *testing.T) {
		t.Parallel()

		cert := &x509.Certificate{Subject: pkix.Name{CommonName: "a-client"}}
		ctx := peer.NewContext(t.Context(), &peer.Peer{
			Addr: addr,
			AuthInfo: credentials.TLSInfo{
				State: tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}},
			},
		})

		require.Equal(t, cert.Subject.String(), fieldMap(getFields(ctx, "/svc/M"))["grpc.peer_subject"])
	})
}

// TestUnaryServerInterceptor covers the successful path: the handler's response
// is returned, the request and response payloads are logged, and the call is
// recorded as completed with an OK code.
func TestUnaryServerInterceptor(t *testing.T) {
	t.Parallel()

	logger, logs := observedLogger()
	interceptor := UnaryServerInterceptor(logger)

	var handlerCtx context.Context
	resp, err := interceptor(
		t.Context(),
		"the-request",
		&grpc.UnaryServerInfo{FullMethod: "/the.Service/TheMethod"},
		func(ctx context.Context, _ any) (any, error) {
			handlerCtx = ctx

			return "the-response", nil
		},
	)

	require.NoError(t, err)
	require.Equal(t, "the-response", resp)

	stored, ok := handlerCtx.Value(fieldKey).([]zapcore.Field)
	require.True(t, ok, "the handler's context carries the fields")
	require.NotEmpty(t, stored)

	messages := logMessages(logs)
	require.Contains(t, messages, "received unary request")
	require.Contains(t, messages, "sending unary response")
	require.Contains(t, messages, "unary call completed")

	completed := entryNamed(t, logs, "unary call completed")
	require.Equal(t, codes.OK.String(), completed.ContextMap()["grpc.code"])
}

// TestUnaryServerInterceptorHandlerError checks a handler failure propagates,
// the response payload is not logged, and the status code is recorded.
func TestUnaryServerInterceptorHandlerError(t *testing.T) {
	t.Parallel()

	logger, logs := observedLogger()
	interceptor := UnaryServerInterceptor(logger)

	handlerErr := status.Error(codes.PermissionDenied, "nope")
	resp, err := interceptor(
		t.Context(),
		"the-request",
		&grpc.UnaryServerInfo{FullMethod: "/the.Service/TheMethod"},
		func(context.Context, any) (any, error) { return nil, handlerErr },
	)

	require.ErrorIs(t, err, handlerErr)
	require.Nil(t, resp)

	messages := logMessages(logs)
	require.Contains(t, messages, "received unary request")
	require.NotContains(t, messages, "sending unary response", "no response is logged when the handler fails")

	completed := entryNamed(t, logs, "unary call completed")
	require.Equal(t, codes.PermissionDenied.String(), completed.ContextMap()["grpc.code"])
}

// TestUnaryServerInterceptorLevels checks the configured levels are honoured:
// raising the level above what the logger records suppresses the entries.
func TestUnaryServerInterceptorLevels(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zapcore.ErrorLevel)
	interceptor := UnaryServerInterceptor(
		zap.New(core),
		WithLeveler(LevelerFunc(func(context.Context, string) zapcore.Level { return zapcore.DebugLevel })),
		WithPayloadLeveler(LevelerFunc(func(context.Context, string) zapcore.Level { return zapcore.DebugLevel })),
	)

	_, err := interceptor(
		t.Context(),
		"req",
		&grpc.UnaryServerInfo{FullMethod: "/svc/M"},
		func(context.Context, any) (any, error) { return "resp", nil },
	)

	require.NoError(t, err)
	require.Zero(t, logs.Len(), "entries below the logger's level are dropped")
}

// TestStreamServerInterceptor covers the successful path and checks the handler
// receives a stream whose context carries the fields.
func TestStreamServerInterceptor(t *testing.T) {
	t.Parallel()

	logger, logs := observedLogger()
	interceptor := StreamServerInterceptor(logger)

	stream := &fakeServerStream{ctx: t.Context()}

	var handlerStream grpc.ServerStream
	err := interceptor(
		"the-service",
		stream,
		&grpc.StreamServerInfo{FullMethod: "/the.Service/TheMethod"},
		func(_ any, s grpc.ServerStream) error {
			handlerStream = s

			return nil
		},
	)

	require.NoError(t, err)
	require.NotNil(t, handlerStream)

	stored, ok := handlerStream.Context().Value(fieldKey).([]zapcore.Field)
	require.True(t, ok, "the wrapped stream's context carries the fields")
	require.NotEmpty(t, stored)

	completed := entryNamed(t, logs, "streaming call completed")
	require.Equal(t, codes.OK.String(), completed.ContextMap()["grpc.code"])
}

// TestStreamServerInterceptorHandlerError checks a handler failure propagates
// and its status code is recorded.
func TestStreamServerInterceptorHandlerError(t *testing.T) {
	t.Parallel()

	logger, logs := observedLogger()
	interceptor := StreamServerInterceptor(logger)

	handlerErr := errors.New("stream failed")
	err := interceptor(
		"the-service",
		&fakeServerStream{ctx: t.Context()},
		&grpc.StreamServerInfo{FullMethod: "/the.Service/TheMethod"},
		func(any, grpc.ServerStream) error { return handlerErr },
	)

	require.ErrorIs(t, err, handlerErr)

	completed := entryNamed(t, logs, "streaming call completed")
	require.Equal(t, codes.Unknown.String(), completed.ContextMap()["grpc.code"],
		"a non-status error is reported as Unknown")
}

// TestServerStreamContext checks the wrapper returns the context it was built
// with rather than the underlying stream's.
func TestServerStreamContext(t *testing.T) {
	t.Parallel()

	type ctxKey struct{}
	wrapped := context.WithValue(t.Context(), ctxKey{}, "value")

	ss := &serverStream{
		ServerStream: &fakeServerStream{ctx: t.Context()},
		context:      wrapped,
	}

	require.Equal(t, "value", ss.Context().Value(ctxKey{}))
}

// TestServerStreamSendMsg checks the message is logged and forwarded, and that
// an error from the underlying stream propagates.
func TestServerStreamSendMsg(t *testing.T) {
	t.Parallel()

	logger, logs := observedLogger()
	inner := &fakeServerStream{}
	ss := &serverStream{
		ServerStream:  inner,
		context:       t.Context(),
		payloadLogger: logger.Named("payload"),
		payloadLevel:  DefaultPayloadLevel,
	}

	require.NoError(t, ss.SendMsg("a-message"))
	require.Equal(t, []any{"a-message"}, inner.sent)
	require.Contains(t, logMessages(logs), "sending stream message")

	inner.sendErr = errors.New("send failed")
	require.ErrorIs(t, ss.SendMsg("another"), inner.sendErr)
}

// TestServerStreamRecvMsg checks the message is logged after it is received,
// so what gets logged is what the underlying stream filled in.
func TestServerStreamRecvMsg(t *testing.T) {
	t.Parallel()

	logger, logs := observedLogger()
	inner := &fakeServerStream{
		recvFill: func(msg any) {
			if s, ok := msg.(*string); ok {
				*s = "filled-by-stream"
			}
		},
	}
	ss := &serverStream{
		ServerStream:  inner,
		context:       t.Context(),
		payloadLogger: logger.Named("payload"),
		payloadLevel:  DefaultPayloadLevel,
	}

	var msg string
	require.NoError(t, ss.RecvMsg(&msg))
	require.Equal(t, "filled-by-stream", msg)
	require.Contains(t, logMessages(logs), "received stream message")

	inner.recvErr = errors.New("recv failed")
	require.ErrorIs(t, ss.RecvMsg(&msg), inner.recvErr)
}

// TestServerStreamPayloadLevelSuppressed checks messages still pass through
// when the payload level is above what the logger records.
func TestServerStreamPayloadLevelSuppressed(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zapcore.ErrorLevel)
	inner := &fakeServerStream{}
	ss := &serverStream{
		ServerStream:  inner,
		context:       t.Context(),
		payloadLogger: zap.New(core).Named("payload"),
		payloadLevel:  zapcore.DebugLevel,
	}

	require.NoError(t, ss.SendMsg("a-message"))
	require.Equal(t, []any{"a-message"}, inner.sent, "the message is still sent")
	require.Zero(t, logs.Len())
}

// fieldMap reduces fields to a name/value map for assertions.
func fieldMap(fields []zapcore.Field) map[string]any {
	enc := zapcore.NewMapObjectEncoder()
	for _, f := range fields {
		f.AddTo(enc)
	}

	return enc.Fields
}

// logMessages returns the message of every recorded entry.
func logMessages(logs *observer.ObservedLogs) []string {
	messages := make([]string, 0, logs.Len())
	for _, e := range logs.All() {
		messages = append(messages, e.Message)
	}

	return messages
}

// entryNamed returns the single entry with the given message.
func entryNamed(t *testing.T, logs *observer.ObservedLogs, message string) observer.LoggedEntry {
	t.Helper()

	found := logs.FilterMessage(message).All()
	require.Len(t, found, 1, "expected exactly one %q entry", message)

	return found[0]
}
