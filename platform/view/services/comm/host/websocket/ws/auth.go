/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ws

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"net/http"

	"github.com/gorilla/websocket"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
)

// expectedPeerIDFromRequest extracts the PeerID from the verified TLS certificate in the request.
// This is used as the cryptographic source of truth for identity binding.
// It relies on the fact that the underlying transport (mTLS) has already
// verified that the client possesses the private key corresponding to this public key.
func expectedPeerIDFromRequest(request *http.Request) (host2.PeerID, error) {
	if request == nil {
		return "", errors.Errorf("missing TLS connection state")
	}
	return peerIDFromConnectionState(request.TLS)
}

// verifyServerPeerID checks that the server certificate verified during conn's TLS handshake
// belongs to expected, the peer ID the client dialed. It is the client-side counterpart of
// expectedPeerIDFromRequest: without it, any certificate that chains to a trusted CA and is
// valid for the dialed host would be accepted as the dialed peer. conn must be a TLS
// connection.
func verifyServerPeerID(conn *websocket.Conn, expected host2.PeerID) error {
	tlsConn, ok := conn.NetConn().(*tls.Conn)
	if !ok {
		return errors.Errorf("missing TLS connection state")
	}
	cs := tlsConn.ConnectionState()
	actual, err := peerIDFromConnectionState(&cs)
	if err != nil {
		return err
	}
	if actual != expected {
		return errors.Errorf("peer identity binding failed: dialed [%s], tls [%s]", expected, actual)
	}
	return nil
}

func peerIDFromConnectionState(cs *tls.ConnectionState) (host2.PeerID, error) {
	if cs == nil {
		return "", errors.Errorf("missing TLS connection state")
	}
	if len(cs.PeerCertificates) == 0 {
		return "", errors.Errorf("missing verified TLS peer certificate")
	}
	return peerIDFromCertificate(cs.PeerCertificates[0])
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
		return base64.StdEncoding.EncodeToString(h[:]), nil
	default:
		return "", errors.Errorf("unsupported public key type [%T]", cert.PublicKey)
	}
}
