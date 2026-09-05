/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc

import (
	"crypto/tls"
	"crypto/x509"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// TLSConfig returns a [tls.Config] for these options, or (nil, nil) when UseTLS is false.
//
// The result serves either role: a TLS server ignores RootCAs and ServerName, and a TLS
// client ignores ClientCAs and ClientAuth. Client authentication has three outcomes:
// RequireClientCert yields [tls.RequireAndVerifyClientCert]; ClientRootCAs without it
// yields [tls.VerifyClientCertIfGiven]; neither yields [tls.NoClientCert].
//
// TimeShift, when set, moves the clock used for certificate validity back by that much.
//
// It returns an error if the keypair does not parse or any CA is not a valid PEM block.
func (so SecureOptions) TLSConfig() (*tls.Config, error) {
	if !so.UseTLS {
		return nil, nil
	}

	suites := so.CipherSuites
	if len(suites) == 0 {
		suites = DefaultTLSCipherSuites
	}
	cfg := &tls.Config{
		CipherSuites:          suites,
		MinVersion:            tls.VersionTLS12,
		MaxVersion:            tls.VersionTLS13,
		ServerName:            so.ServerNameOverride,
		VerifyPeerCertificate: so.VerifyCertificate,
	}
	if so.TimeShift > 0 {
		// Verify peer certificates against a clock shifted into the past, for deployments
		// whose certificates are not yet valid by this host's clock.
		cfg.Time = func() time.Time { return time.Now().Add(-so.TimeShift) }
	}

	if len(so.Certificate) > 0 || len(so.Key) > 0 {
		cert, err := tls.X509KeyPair(so.Certificate, so.Key)
		if err != nil {
			return nil, errors.Wrap(err, "failed loading TLS key pair")
		}
		cfg.Certificates = []tls.Certificate{cert}
	}

	if len(so.ServerRootCAs) > 0 {
		pool, err := certPool(so.ServerRootCAs)
		if err != nil {
			return nil, errors.WithMessage(err, "server root CAs")
		}
		cfg.RootCAs = pool
	}

	// Three states, not two. RequireClientCert demands a certificate; client root CAs
	// without it mean "verify one if offered", which is what the web listener has always
	// done and must keep doing.
	if len(so.ClientRootCAs) > 0 {
		pool, err := certPool(so.ClientRootCAs)
		if err != nil {
			return nil, errors.WithMessage(err, "client root CAs")
		}
		cfg.ClientCAs = pool
		cfg.ClientAuth = tls.VerifyClientCertIfGiven
	}
	if so.RequireClientCert {
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
	}

	return cfg, nil
}

func certPool(pems [][]byte) (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	for _, p := range pems {
		if !pool.AppendCertsFromPEM(p) {
			return nil, errors.New("failed to append certificate: not a valid PEM block")
		}
	}
	return pool, nil
}
