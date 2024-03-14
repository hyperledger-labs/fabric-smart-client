/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest

import (
	"crypto/tls"
	"crypto/x509"
	"os"

	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/pkg/errors"
)

type client struct {
	tlsConfig *tls.Config
	nodeID    host2.PeerID
}

func newClient(nodeID host2.PeerID, rootCAs []string, tlsEnabled bool) (*client, error) {
	logger.Infof("Creating p2p client for node ID [%s] with tlsEnabled = %v", nodeID, tlsEnabled)
	caCertPool, err := newRootCACertPool(rootCAs)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read root CA certs")
	}
	c := &client{
		tlsConfig: &tls.Config{
			InsecureSkipVerify: tlsEnabled && caCertPool == nil,
			RootCAs:            caCertPool,
		},
		nodeID: nodeID,
	}
	logger.Infof("Created p2p client for node ID [%s] with %d root CAs and InsecureSkipVerify = %v", nodeID, len(rootCAs), c.tlsConfig.InsecureSkipVerify)
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

func (c *client) OpenStream(peerAddress host2.PeerIPAddress, peerID host2.PeerID) (host2.P2PStream, error) {
	return newClientStream(peerAddress, c.nodeID, peerID, c.tlsConfig)
}
