/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"net/http"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/mr-tron/base58/base58"
)

// expectedPeerIDFromRequest extracts the PeerID from the verified TLS certificate in the request.
// This is used as the cryptographic source of truth for identity binding.
// It relies on the fact that the underlying transport (mTLS) has already
// verified that the client possesses the private key corresponding to this public key.
func expectedPeerIDFromRequest(request *http.Request) (host2.PeerID, error) {
	if request == nil || request.TLS == nil {
		return "", errors.Errorf("missing TLS connection state")
	}
	if len(request.TLS.PeerCertificates) == 0 {
		return "", errors.Errorf("missing verified TLS peer certificate")
	}

	return peerIDFromCertificate(request.TLS.PeerCertificates[0])
}

func peerIDFromCertificate(cert *x509.Certificate) (host2.PeerID, error) {
	if cert == nil {
		return "", errors.Errorf("nil certificate")
	}

	switch k := cert.PublicKey.(type) {
	case *ecdsa.PublicKey:
		marshaledPubKey, err := x509.MarshalPKIXPublicKey(k)
		if err != nil {
			return "", errors.Wrapf(err, "failed to marshal public key")
		}
		h := sha256.Sum256(marshaledPubKey)
		return host2.PeerID(base58.Encode(h[:])), nil
	default:
		return "", errors.Errorf("unsupported public key type [%T]", cert.PublicKey)
	}
}
