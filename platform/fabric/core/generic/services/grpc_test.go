/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package services

import (
	"context"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"
	grpc2 "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc/tlsgen"
)

func newTestGRPCClient(t *testing.T, secOpts grpc.SecureOptions) *grpc.Client {
	t.Helper()
	c, err := grpc.NewGRPCClient(grpc.ClientConfig{SecOpts: secOpts, Timeout: time.Second, AsyncConnect: true})
	require.NoError(t, err)
	return c
}

func TestGRPCClient(t *testing.T) {
	t.Parallel()

	t.Run("clients are created without a running server", func(t *testing.T) {
		t.Parallel()
		c := NewGRPCClient(newTestGRPCClient(t, grpc.SecureOptions{}), "127.0.0.1:1", fakeSigner{}.Sign)
		defer c.Close()

		require.Equal(t, "127.0.0.1:1", c.Address())
		require.Empty(t, c.Certificate().Certificate, "no client certificate without TLS")

		ec, err := c.EndorserClient()
		require.NoError(t, err)
		require.NotNil(t, ec)
		dc, err := c.DeliverClient()
		require.NoError(t, err)
		require.NotNil(t, dc)
		oc, err := c.OrdererClient()
		require.NoError(t, err)
		require.NotNil(t, oc)
		disc, err := c.DiscoveryClient()
		require.NoError(t, err)
		require.NotNil(t, disc)
	})

	t.Run("clients fail without an address", func(t *testing.T) {
		t.Parallel()
		c := NewGRPCClient(newTestGRPCClient(t, grpc.SecureOptions{}), "", fakeSigner{}.Sign)

		_, err := c.EndorserClient()
		require.ErrorContains(t, err, "address is empty")
		_, err = c.DeliverClient()
		require.ErrorContains(t, err, "address is empty")
		_, err = c.OrdererClient()
		require.ErrorContains(t, err, "address is empty")
	})

	t.Run("Certificate returns the client TLS certificate", func(t *testing.T) {
		t.Parallel()
		ca, err := tlsgen.NewCA()
		require.NoError(t, err)
		kp, err := ca.NewClientCertKeyPair()
		require.NoError(t, err)
		c := NewGRPCClient(newTestGRPCClient(t, grpc.SecureOptions{
			UseTLS:            true,
			RequireClientCert: true,
			Certificate:       kp.Cert,
			Key:               kp.Key,
			ServerRootCAs:     [][]byte{ca.CertBytes()},
		}), "127.0.0.1:1", fakeSigner{}.Sign)

		cert := c.Certificate()
		require.Len(t, cert.Certificate, 1)
		require.Equal(t, kp.TLSCert.Raw, cert.Certificate[0])
	})

	t.Run("Close closes the connections", func(t *testing.T) {
		t.Parallel()
		c := NewGRPCClient(newTestGRPCClient(t, grpc.SecureOptions{}), "127.0.0.1:1", fakeSigner{}.Sign)
		ec, err := c.EndorserClient()
		require.NoError(t, err)

		c.Close()
		_, err = ec.ProcessProposal(context.Background(), &peer.SignedProposal{})
		require.Equal(t, codes.Canceled, status.Code(err), "calls on a closed connection are cancelled: %v", err)
	})
}

func TestLazyGRPCClient(t *testing.T) {
	t.Parallel()

	var conns []*grpc2.ClientConn
	connect := func() (*grpc2.ClientConn, error) {
		cc, err := grpc2.NewClient("passthrough:///fake", grpc2.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			conns = append(conns, cc)
		}
		return cc, err
	}
	c := NewLazyGRPCClient(newClient(newTestGRPCClient(t, grpc.SecureOptions{}), "fake", fakeSigner{}.Sign, connect))

	_, err := c.EndorserClient()
	require.NoError(t, err)
	_, err = c.DeliverClient()
	require.NoError(t, err)
	_, err = c.OrdererClient()
	require.NoError(t, err)
	require.Len(t, conns, 1, "the connection is shared")

	require.NoError(t, c.Reset())
	require.Equal(t, connectivity.Shutdown, conns[0].GetState(), "Reset closes the connection")

	_, err = c.EndorserClient()
	require.NoError(t, err)
	require.Len(t, conns, 2, "a reset client reconnects")
	require.NoError(t, c.Reset())
}
