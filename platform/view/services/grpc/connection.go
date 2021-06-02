/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc

import (
	"crypto/tls"
	"crypto/x509"
	"sync"

	"google.golang.org/grpc/credentials"
)

// CredentialSupport type manages credentials used for gRPC client connections
type CredentialSupport struct {
	mutex             sync.RWMutex
	appRootCAsByChain map[string][][]byte
	serverRootCAs     [][]byte
	clientCert        tls.Certificate
}

// NewCredentialSupport creates a CredentialSupport instance.
func NewCredentialSupport(rootCAs ...[]byte) *CredentialSupport {
	return &CredentialSupport{
		appRootCAsByChain: make(map[string][][]byte),
		serverRootCAs:     rootCAs,
	}
}

// SetClientCertificate sets the tls.Certificate to use for gRPC client
// connections
func (cs *CredentialSupport) SetClientCertificate(cert tls.Certificate) {
	cs.mutex.Lock()
	cs.clientCert = cert
	cs.mutex.Unlock()
}

// getClientCertificate returns the client certificate of the CredentialSupport
func (cs *CredentialSupport) GetClientCertificate() tls.Certificate {
	cs.mutex.RLock()
	defer cs.mutex.RUnlock()
	return cs.clientCert
}

// GetPeerCredentials returns gRPC transport credentials for use by gRPC
// clients which communicate with remote peer endpoints.
func (cs *CredentialSupport) GetPeerCredentials() credentials.TransportCredentials {
	cs.mutex.RLock()
	defer cs.mutex.RUnlock()

	var appRootCAs [][]byte
	appRootCAs = append(appRootCAs, cs.serverRootCAs...)
	for _, appRootCA := range cs.appRootCAsByChain {
		appRootCAs = append(appRootCAs, appRootCA...)
	}

	certPool := x509.NewCertPool()
	for _, appRootCA := range appRootCAs {
		err := AddPemToCertPool(appRootCA, certPool)
		if err != nil {
			commLogger.Warningf("Failed adding certificates to peer's client TLS trust pool: %s", err)
		}
	}

	return credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cs.clientCert},
		RootCAs:      certPool,
	})
}

func (cs *CredentialSupport) AppRootCAsByChain() map[string][][]byte {
	cs.mutex.RLock()
	defer cs.mutex.RUnlock()
	return cs.appRootCAsByChain
}
