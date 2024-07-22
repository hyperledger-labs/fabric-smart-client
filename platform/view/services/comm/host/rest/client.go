/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"os"

	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/pkg/errors"
)

type clientStreamProvider interface {
	NewClientStream(info host2.StreamInfo, ctx context.Context, src host2.PeerID, config *tls.Config) (host2.P2PStream, error)
}

type client struct {
	tlsConfig      *tls.Config
	nodeID         host2.PeerID
	streamProvider clientStreamProvider
}

func newClient(streamProvider clientStreamProvider, nodeID host2.PeerID, rootCAs []string, tlsEnabled bool) (*client, error) {
	logger.Debugf("Creating p2p client for node ID [%s] with tlsEnabled = %v", nodeID, tlsEnabled)
	caCertPool, err := newRootCACertPool(rootCAs)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read root CA certs")
	}
	c := &client{
		tlsConfig: &tls.Config{
			InsecureSkipVerify: tlsEnabled && caCertPool == nil,
			RootCAs:            caCertPool,
		},
		nodeID:         nodeID,
		streamProvider: streamProvider,
	}
	logger.Debugf("Created p2p client for node ID [%s] with %d root CAs and InsecureSkipVerify = %v", nodeID, len(rootCAs), c.tlsConfig.InsecureSkipVerify)
	return c, nil
}

func newRootCACertPool(rootCAs []string) (*x509.CertPool, error) {
	if len(rootCAs) == 0 {
		return nil, nil
	}
	caCertPool := x509.NewCertPool()
	for _, rootCA := range rootCAs {
		caCert, err := os.ReadFile(rootCA)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read PEM cert in [%s]", caCert)
		}
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, errors.Errorf("failed to append cert from [%s]", caCert)
		}
	}
	return caCertPool, nil
}

func (c *client) OpenStream(info host2.StreamInfo, ctx context.Context) (host2.P2PStream, error) {
	return c.streamProvider.NewClientStream(info, ctx, c.nodeID, c.tlsConfig)
}
