/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc

import (
	"cmp"
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
		MinVersion:            cmp.Or(so.MinVersion, uint16(tls.VersionTLS12)),
		MaxVersion:            cmp.Or(so.MaxVersion, uint16(tls.VersionTLS13)),
		ServerName:            so.ServerNameOverride,
		VerifyPeerCertificate: so.VerifyCertificate,
		// A resumed session does not re-verify the peer's chain, so a custom
		// VerifyPeerCertificate would never run on one and the check could be skipped by
		// resuming. Disabling tickets forces a full handshake every time, which is what
		// grpc/server.go and the gRPC client have always done.
		SessionTicketsDisabled: true,
	}

	if err := validateVersionRange(cfg.MinVersion, cfg.MaxVersion); err != nil {
		return nil, err
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

	if len(so.ClientRootCAs) > 0 {
		pool, err := certPool(so.ClientRootCAs)
		if err != nil {
			return nil, errors.WithMessage(err, "client root CAs")
		}
		cfg.ClientCAs = pool
	}
	cfg.ClientAuth = so.clientAuth()

	return cfg, nil
}

// clientAuth is the client authentication a TLS server applies for these options. It has
// three states, not two: RequireClientCert demands and verifies a certificate, ClientRootCAs
// alone verify one only if the client offers it, and neither requests none. The fsc.grpc and
// fsc.web listeners both apply it, so one configuration means the same thing on either.
func (so SecureOptions) clientAuth() tls.ClientAuthType {
	switch {
	case so.RequireClientCert:
		return tls.RequireAndVerifyClientCert
	case len(so.ClientRootCAs) > 0:
		return tls.VerifyClientCertIfGiven
	default:
		return tls.NoClientCert
	}
}

// validateVersionRange rejects a floor below the lowest supported TLS version, and a ceiling
// below the floor.
//
// A ceiling below the floor makes every handshake fail at connect time with an opaque error;
// naming it here fails at startup instead. Nothing below 1.2 is offered: the cipher suite
// list has no forward-secrecy-free suites left for 1.0/1.1 anyway.
func validateVersionRange(minVersion, maxVersion uint16) error {
	if minVersion < tls.VersionTLS12 {
		return errors.Errorf("minVersion [%d] is below the lowest supported TLS version [%d]",
			minVersion, tls.VersionTLS12)
	}
	if maxVersion < minVersion {
		return errors.Errorf("maxVersion [%d] is below minVersion [%d]", maxVersion, minVersion)
	}
	return nil
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
