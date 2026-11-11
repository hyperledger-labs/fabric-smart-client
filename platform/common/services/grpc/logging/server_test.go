/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
)

// fakeServerStream is a grpc.ServerStream that only carries a context.
type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f *fakeServerStream) Context() context.Context { return f.ctx }

// TestUnaryServerInterceptor checks the handler's result is returned and one Debug line
// records the call.
func TestUnaryServerInterceptor(t *testing.T) {
	t.Parallel()
	logger, r := logging.NewTestLogger(t)
	handlerErr := status.Error(codes.PermissionDenied, "nope")

	for _, tc := range []struct {
		name string
		resp any
		err  error
		code codes.Code
	}{
		{"ok", "the-response", nil, codes.OK},
		{"handler error", nil, handlerErr, codes.PermissionDenied},
	} {
		r.Reset()
		resp, err := UnaryServerInterceptor(logger)(
			t.Context(),
			"the-request",
			&grpc.UnaryServerInfo{FullMethod: "/the.Service/TheMethod"},
			func(context.Context, any) (any, error) { return tc.resp, tc.err },
		)

		assert.Equal(t, tc.resp, resp, tc.name)
		assert.Equal(t, tc.err, err, tc.name) //nolint:testifylint // the handler's error is returned as is
		entries := r.Entries()
		require.Len(t, entries, 1, tc.name)
		assert.Contains(t, entries[0], " DEBU ", tc.name)
		assert.Contains(t, entries[0], "unary call completed", tc.name)
		assert.Contains(t, entries[0], "grpc.service=the.Service grpc.method=TheMethod", tc.name)
		assert.Contains(t, entries[0], "grpc.code="+tc.code.String(), tc.name)
		assert.Contains(t, entries[0], "grpc.call_duration=", tc.name)
	}
	assert.Contains(t, r.Entries()[0], `error="rpc error: code = PermissionDenied desc = nope"`)
}

// TestStreamServerInterceptor checks the handler gets the original stream, and one Debug
// line records the call; a non-status error is reported as Unknown.
func TestStreamServerInterceptor(t *testing.T) {
	t.Parallel()
	logger, r := logging.NewTestLogger(t)
	stream := &fakeServerStream{ctx: t.Context()}
	handlerErr := errors.New("stream failed")

	var got grpc.ServerStream
	err := StreamServerInterceptor(logger)(
		"the-service",
		stream,
		&grpc.StreamServerInfo{FullMethod: "/the.Service/TheMethod"},
		func(_ any, s grpc.ServerStream) error {
			got = s
			return handlerErr
		},
	)

	require.ErrorIs(t, err, handlerErr)
	assert.Same(t, stream, got)
	entries := r.Entries()
	require.Len(t, entries, 1)
	assert.Contains(t, entries[0], " DEBU ")
	assert.Contains(t, entries[0], "streaming call completed")
	assert.Contains(t, entries[0], "grpc.service=the.Service grpc.method=TheMethod")
	assert.Contains(t, entries[0], "grpc.code=Unknown")
	assert.Contains(t, entries[0], `error="stream failed"`)
}

// TestInterceptorsNothingAtInfo checks that nothing is logged when Debug is off.
func TestInterceptorsNothingAtInfo(t *testing.T) { //nolint:paralleltest // changes the global logging configuration
	var buf bytes.Buffer
	require.NoError(t, logging.Init(logging.Config{LogSpec: "info", Writer: &buf}))
	t.Cleanup(func() { require.NoError(t, logging.Init(logging.Config{Writer: os.Stderr})) })
	logger := logging.MustGetLogger()

	_, err := UnaryServerInterceptor(logger)(t.Context(), "req", &grpc.UnaryServerInfo{FullMethod: "/svc/M"},
		func(context.Context, any) (any, error) { return "resp", nil })
	require.NoError(t, err)
	err = StreamServerInterceptor(logger)("svc", &fakeServerStream{ctx: t.Context()}, &grpc.StreamServerInfo{FullMethod: "/svc/M"},
		func(any, grpc.ServerStream) error { return nil })
	require.NoError(t, err)

	assert.Empty(t, buf.String())
}

// TestCallFields covers the optional fields: a malformed method name, a deadline, and the
// peer address and TLS client certificate subject.
func TestCallFields(t *testing.T) {
	t.Parallel()
	fields := func(ctx context.Context, method string) map[any]any {
		kv := callFields(ctx, method, time.Now(), nil)
		m := map[any]any{}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return m
	}

	plain := fields(t.Context(), "the.Service.TheMethod")
	assert.NotContains(t, plain, "grpc.service", "a method name without two slashes is not split")
	assert.NotContains(t, plain, "grpc.request_deadline")
	assert.NotContains(t, plain, "grpc.peer_address")
	assert.NotContains(t, plain, "error")
	assert.Equal(t, "OK", plain["grpc.code"])

	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	assert.Contains(t, fields(ctx, "/s/m"), "grpc.request_deadline")

	addr := &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 7051}
	cert := &x509.Certificate{Subject: pkix.Name{CommonName: "a-client"}}
	ctx = peer.NewContext(t.Context(), &peer.Peer{
		Addr:     addr,
		AuthInfo: credentials.TLSInfo{State: tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}},
	})
	withPeer := fields(ctx, "/s/m")
	assert.Equal(t, addr.String(), withPeer["grpc.peer_address"])
	assert.Equal(t, cert.Subject.String(), withPeer["grpc.peer_subject"])
}
