/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
)

// NewConfigFromProperties builds a configuration from file paths, reading each one eagerly.
// The certificate serves as both the transport certificate and the node's identity, which
// is what tests of a single host want; production resolves the two separately in
// [NewConfig].
func NewConfigFromProperties(listenAddress, privateKeyPath, certPath string, serverRootCAs, clientRootCAs []string, clientAuthRequired bool, maxSubConns int, corsAllowedOrigins []string) (*config, error) {
	read := func(path string) ([]byte, error) {
		if path == "" {
			return nil, nil
		}
		return os.ReadFile(path)
	}
	readAll := func(paths []string) ([][]byte, error) {
		out := make([][]byte, 0, len(paths))
		for _, p := range paths {
			b, err := read(p)
			if err != nil {
				return nil, err
			}
			if len(b) > 0 {
				out = append(out, b)
			}
		}
		return out, nil
	}

	cert, err := read(certPath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read cert [%s]", certPath)
	}
	key, err := read(privateKeyPath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read private key [%s]", privateKeyPath)
	}
	clientRootCAsRaw, err := readAll(clientRootCAs)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read client root CAs")
	}
	serverRootCAsRaw, err := readAll(serverRootCAs)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read server root CAs")
	}

	return &config{
		listenAddress:    listenAddress,
		identityCertPath: certPath,
		serverTLS: grpc.SecureOptions{
			UseTLS: true, Certificate: cert, Key: key,
			RequireClientCert: clientAuthRequired, ClientRootCAs: clientRootCAsRaw,
		},
		clientTLS: grpc.SecureOptions{
			UseTLS: true, Certificate: cert, Key: key,
			ServerRootCAs: serverRootCAsRaw,
		},
		maxSubConns:        maxSubConns,
		corsAllowedOrigins: corsAllowedOrigins,
	}, nil
}

// GenerateTestCert generates a self-signed certificate for testing purposes.
func GenerateTestCert(cn string) (cert, key []byte, err error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: cn,
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Hour * 24),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"localhost"},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})

	return certPEM, keyPEM, nil
}
