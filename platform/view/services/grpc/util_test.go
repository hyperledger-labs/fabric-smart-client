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

	grpc3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

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

type nonTLSConnection struct {
}

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
