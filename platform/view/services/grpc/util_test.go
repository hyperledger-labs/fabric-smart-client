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

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"

	grpc3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

func TestExtractCertificateHashFromContext(t *testing.T) {
	t.Parallel()
	assert.Nil(t, grpc3.ExtractCertificateHashFromContext(context.Background()))

	p := &peer.Peer{}
	ctx := peer.NewContext(context.Background(), p)
	assert.Nil(t, grpc3.ExtractCertificateHashFromContext(ctx))

	p.AuthInfo = &nonTLSConnection{}
	ctx = peer.NewContext(context.Background(), p)
	assert.Nil(t, grpc3.ExtractCertificateHashFromContext(ctx))

	p.AuthInfo = credentials.TLSInfo{}
	ctx = peer.NewContext(context.Background(), p)
	assert.Nil(t, grpc3.ExtractCertificateHashFromContext(ctx))

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
	assert.Equal(t, h.Sum(nil), grpc3.ExtractCertificateHashFromContext(ctx))
}

type nonTLSConnection struct {
}

func (*nonTLSConnection) AuthType() string {
	return ""
}

func TestBindingInspectorBadInit(t *testing.T) {
	t.Parallel()
	assert.Panics(t, func() {
		grpc3.NewBindingInspector(false, nil)
	})
}

func TestGetLocalIP(t *testing.T) {
	ip, err := grpc3.GetLocalIP()
	assert.NoError(t, err)
	t.Log(ip)
}
