/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc_test

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/peer"

	grpc3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

func waitServerReady(t *testing.T, address string, dialOptions ...grpc.DialOption) {
	t.Helper()
	require.Eventually(t, func() bool {
		ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
		defer cancel()
		conn, err := grpc.NewClient(address, dialOptions...)
		if err != nil {
			return false
		}
		defer func() { _ = conn.Close() }()
		healthClient := grpc_health_v1.NewHealthClient(conn)
		resp, err := healthClient.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
		if err != nil {
			return false
		}
		return resp.Status == grpc_health_v1.HealthCheckResponse_SERVING
	}, 10*time.Second, 20*time.Millisecond)
}

func TestExtractCertificateHashFromContext(t *testing.T) {
	t.Parallel()
	require.Nil(t, grpc3.ExtractCertificateHashFromContext(context.Background()))

	p := &peer.Peer{}
	ctx := peer.NewContext(context.Background(), p)
	require.Nil(t, grpc3.ExtractCertificateHashFromContext(ctx))

	p.AuthInfo = &nonTLSConnection{}
	ctx = peer.NewContext(context.Background(), p)
	require.Nil(t, grpc3.ExtractCertificateHashFromContext(ctx))

	p.AuthInfo = credentials.TLSInfo{}
	ctx = peer.NewContext(context.Background(), p)
	require.Nil(t, grpc3.ExtractCertificateHashFromContext(ctx))

	p.AuthInfo = credentials.TLSInfo{
		State: tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{
				{Raw: []byte{1, 2, 3}},
			},
		},
	}
	ctx = peer.NewContext(context.Background(), p)
	h := sha256.New()
	h.Write([]byte{1, 2, 3})
	require.Equal(t, h.Sum(nil), grpc3.ExtractCertificateHashFromContext(ctx))
}

type nonTLSConnection struct{}

func (*nonTLSConnection) AuthType() string {
	return ""
}

func TestBindingInspectorBadInit(t *testing.T) {
	t.Parallel()
	// nil extractor is only a programming error when mutualTLS is enabled,
	// because the extractor is not called in noop mode.
	require.NotPanics(t, func() {
		grpc3.NewBindingInspector(false, nil)
	})
	require.Panics(t, func() {
		grpc3.NewBindingInspector(true, nil)
	})
}

func TestGetLocalIP(t *testing.T) {
	t.Parallel()
	ip, err := grpc3.GetLocalIP()
	require.NoError(t, err)
	t.Log(ip)
}
