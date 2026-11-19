/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsc

import (
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"time"

	nwonode "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// RenewNodeIdentity rotates peer's FSC identity, as an operator does: it writes a new key, and a
// certificate for it signed by the CA of peer's organization with the subject, names and expiry
// of the current certificate, over peer's identity files. Peer's own core.yaml and the
// endpoint.resolvers entry for peer in every other node's core.yaml point at these files, and
// websocket P2P takes its TLS key pair from them, so every node uses the new identity once it
// restarts. After peer restarted, RefreshNodeClients makes the platform's view clients sign with
// the new identity.
func (p *Platform) RenewNodeIdentity(peer *nwonode.Peer) error {
	certPath, keyPath := p.LocalMSPIdentityCert(peer), p.LocalMSPPrivateKey(peer)
	current, err := os.ReadFile(certPath)
	if err != nil {
		return errors.Wrapf(err, "failed reading the identity of [%s]", peer.Name)
	}
	leaf, err := parseCert(current)
	if err != nil {
		return errors.Wrapf(err, "failed parsing the identity of [%s]", peer.Name)
	}
	org := p.Organization(peer.Organization)
	cert, key, err := issueCert(filepath.Join(p.orgCryptoDir(peer), "ca"), fmt.Sprintf("ca.%s-cert.pem", org.Domain), &x509.Certificate{
		Subject:     leaf.Subject,
		NotBefore:   time.Now().Add(-time.Minute),
		NotAfter:    leaf.NotAfter,
		KeyUsage:    leaf.KeyUsage,
		ExtKeyUsage: leaf.ExtKeyUsage,
		DNSNames:    leaf.DNSNames,
		IPAddresses: leaf.IPAddresses,
	})
	if err != nil {
		return errors.WithMessagef(err, "failed renewing the identity of [%s]", peer.Name)
	}
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		return errors.Wrapf(err, "failed writing the identity key of [%s]", peer.Name)
	}
	return errors.Wrapf(os.WriteFile(certPath, cert, 0o600), "failed writing the identity of [%s]", peer.Name)
}

// RefreshNodeClients reads the identity of the FSC node name again and replaces its view
// clients and its view identity in the context, as the platform set them when the network
// started. Call it after RenewNodeIdentity and a restart of the node.
func (p *Platform) RefreshNodeClients(name string) {
	for _, replica := range p.Peers {
		if replica.Name == name {
			p.setClients(replica)
		}
	}
}
