/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsc

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	dto "github.com/prometheus/client_model/go"

	nwonode "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// TLSCertNotAfterMetric is the gauge of the expiry of the TLS certificate each listener serves.
const TLSCertNotAfterMetric = "fsc_tls_cert_not_after"

// ParseTLSCertNotAfter returns the fsc_tls_cert_not_after series of families, keyed by the value
// of their listener label (grpc, web, metrics), in UTC, so they compare with == against a
// certificate's NotAfter.
func ParseTLSCertNotAfter(families map[string]*dto.MetricFamily) map[string]time.Time {
	expiry := map[string]time.Time{}
	for _, m := range families[TLSCertNotAfterMetric].GetMetric() {
		for _, l := range m.GetLabel() {
			if l.GetName() == "listener" {
				expiry[l.GetValue()] = time.Unix(int64(m.GetGauge().GetValue()), 0).UTC()
			}
		}
	}
	return expiry
}

// TLSPair is a node's TLS certificate and key, in PEM.
type TLSPair struct {
	Cert, Key []byte
}

// NodeTLSPair returns the TLS certificate and key peer's listeners serve.
func (p *Platform) NodeTLSPair(peer *nwonode.Peer) (TLSPair, error) {
	dir := p.NodeLocalTLSDir(peer)
	cert, err := os.ReadFile(filepath.Join(dir, "server.crt"))
	if err != nil {
		return TLSPair{}, errors.Wrapf(err, "failed reading the TLS certificate of [%s]", peer.Name)
	}
	key, err := os.ReadFile(filepath.Join(dir, "server.key"))
	if err != nil {
		return TLSPair{}, errors.Wrapf(err, "failed reading the TLS key of [%s]", peer.Name)
	}
	return TLSPair{Cert: cert, Key: key}, nil
}

// WriteNodeTLSPair overwrites the TLS certificate and key peer's listeners serve with pair.
func (p *Platform) WriteNodeTLSPair(peer *nwonode.Peer, pair TLSPair) error {
	dir := p.NodeLocalTLSDir(peer)
	if err := os.WriteFile(filepath.Join(dir, "server.key"), pair.Key, 0o600); err != nil {
		return errors.Wrapf(err, "failed writing the TLS key of [%s]", peer.Name)
	}
	return errors.Wrapf(os.WriteFile(filepath.Join(dir, "server.crt"), pair.Cert, 0o600), "failed writing the TLS certificate of [%s]", peer.Name)
}

// RenewNodeTLSPair returns a new TLS certificate and key for peer, with a new key, signed by the
// TLS CA of its organization and valid until notAfter. It keeps the subject and the names of the
// certificate peer serves, so every client that verifies peer today verifies the renewed one.
func (p *Platform) RenewNodeTLSPair(peer *nwonode.Peer, notAfter time.Time) (TLSPair, error) {
	current, err := p.NodeTLSPair(peer)
	if err != nil {
		return TLSPair{}, err
	}
	leaf, err := parseCert(current.Cert)
	if err != nil {
		return TLSPair{}, errors.Wrapf(err, "failed parsing the TLS certificate of [%s]", peer.Name)
	}
	caDir := filepath.Join(p.orgCryptoDir(peer), "tlsca")
	org := p.Organization(peer.Organization)
	caCertPEM, err := os.ReadFile(filepath.Join(caDir, fmt.Sprintf("tlsca.%s-cert.pem", org.Domain)))
	if err != nil {
		return TLSPair{}, errors.Wrapf(err, "failed reading the TLS CA of [%s]", peer.Organization)
	}
	caCert, err := parseCert(caCertPEM)
	if err != nil {
		return TLSPair{}, errors.Wrapf(err, "failed parsing the TLS CA of [%s]", peer.Organization)
	}
	caKeyPEM, err := os.ReadFile(filepath.Join(caDir, "priv_sk"))
	if err != nil {
		return TLSPair{}, errors.Wrapf(err, "failed reading the TLS CA key of [%s]", peer.Organization)
	}
	caKey, err := parseKey(caKeyPEM)
	if err != nil {
		return TLSPair{}, errors.Wrapf(err, "failed parsing the TLS CA key of [%s]", peer.Organization)
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return TLSPair{}, errors.Wrap(err, "failed generating a TLS key")
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return TLSPair{}, errors.Wrap(err, "failed generating a serial number")
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      leaf.Subject,
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     notAfter,
		KeyUsage:     leaf.KeyUsage,
		ExtKeyUsage:  leaf.ExtKeyUsage,
		DNSNames:     leaf.DNSNames,
		IPAddresses:  leaf.IPAddresses,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	if err != nil {
		return TLSPair{}, errors.Wrapf(err, "failed signing a TLS certificate for [%s]", peer.Name)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return TLSPair{}, errors.Wrap(err, "failed encoding the TLS key")
	}
	return TLSPair{
		Cert: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		Key:  pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
	}, nil
}

// orgCryptoDir is the crypto directory of peer's organization.
func (p *Platform) orgCryptoDir(peer *nwonode.Peer) string {
	return filepath.Join(p.CryptoPath(), "peerOrganizations", p.Organization(peer.Organization).Domain)
}

func parseCert(raw []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("no PEM block")
	}
	return x509.ParseCertificate(block.Bytes)
}

func parseKey(raw []byte) (any, error) {
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("no PEM block")
	}
	return x509.ParsePKCS8PrivateKey(block.Bytes)
}
