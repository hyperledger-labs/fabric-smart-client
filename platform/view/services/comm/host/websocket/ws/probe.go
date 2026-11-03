/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ws

import (
	"context"
	"crypto/tls"
	"net/url"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/client"
)

// ReadPreamble dials the websocket P2P endpoint of peerID at address, checks that the server's
// verified TLS certificate belongs to peerID, reads the preamble from the upgrade response and
// closes the websocket. It opens no sub-connection, so the peer starts no session. config must
// be the node's client TLS configuration, with ServerName set.
//
// The preamble is returned undecoded so that the caller can tell a peer it cannot reach from a
// peer it reaches but would refuse; decode it with preamble.Accept. It is nil for a peer that
// runs a release without preambles. An error wrapping ErrBadPreamble means the peer was reached
// but sent a preamble header a node refuses.
func ReadPreamble(ctx context.Context, peerID host.PeerID, address string, config *tls.Config) ([]byte, error) {
	if config == nil {
		return nil, errors.New("websocket p2p requires TLS")
	}
	u := url.URL{Scheme: schemes[true], Host: address, Path: "/p2p"}
	conn, header, err := client.OpenWSClientConnWithHeader(ctx, u.String(), config, 0)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open websocket to [%s]", address)
	}
	defer func() { _ = conn.Close() }()
	if err := verifyServerPeerID(conn, peerID); err != nil {
		return nil, err
	}
	raw, err := preambleFromHeader(header)
	if err != nil {
		return nil, errors.Wrap(ErrBadPreamble, err.Error())
	}
	if len(raw) == 0 {
		return nil, nil
	}
	return raw, nil
}

// ErrBadPreamble is wrapped by ReadPreamble when the peer was reached but its preamble header is
// oversized or malformed, so a node would refuse it.
var ErrBadPreamble = errors.New("bad preamble header")
