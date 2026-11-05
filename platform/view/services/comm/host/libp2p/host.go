/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package libp2p

import (
	"context"
	"strings"

	libp2plogging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	libp2phost "github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2ptls "github.com/libp2p/go-libp2p/p2p/security/tls"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	"github.com/multiformats/go-multiaddr"
	"go.uber.org/zap/zapcore"

	fscutils "github.com/hyperledger-labs/fabric-smart-client/pkg/utils"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	commhost "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/utils"
)

var logger = logging.MustGetLogger()

const (
	// viewProtocolV2 streams start with a preamble frame in each direction; see
	// exchangePreamble. A node dials it first.
	viewProtocolV2 protocol.ID = "/fsc/view/2.0.0"
	// viewProtocol streams carry view packets only. A node falls back to it for a peer of a
	// release that registers no viewProtocolV2; libp2p negotiates the protocol before any
	// byte of FSC data, so such a peer would otherwise refuse the stream unreadably.
	viewProtocol protocol.ID = "/fsc/view/1.0.0"
)

type host struct {
	libp2phost.Host

	// peerID caches libp2phost.Host.ID().String() since the underlying libp2p peer ID
	// never changes over the host's lifetime, avoiding repeated base58 encoding
	// on every PeerID() call.
	peerID  commhost.PeerID
	metrics *metrics
}

// newLibP2PHost creates a host that listens on config.ListenAddress. It finds no peers on its
// own: NewStream dials the address its caller passes.
func newLibP2PHost(config libp2pConfig, priv crypto.PrivKey, metrics *metrics) (*host, error) {
	listenAddress := config.ListenAddress()
	logger.Debugf("libp2p: Creating new host at [%s]...", listenAddress)
	addr, err := multiaddr.NewMultiaddr(listenAddress)
	if err != nil {
		return nil, err
	}

	connManager, err := connmgr.NewConnManager(
		config.ConnManagerLowWater(),
		config.ConnManagerHighWater(),
		connmgr.WithGracePeriod(config.ConnManagerGracePeriod()),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed creating conn manager for libp2p host")
	}
	opts := []libp2p.Option{
		libp2p.ListenAddrs(addr),
		libp2p.Identity(priv),
		// support TLS connections
		libp2p.Security(libp2ptls.ID, libp2ptls.New),
		// support noise connections
		libp2p.Security(noise.ID, noise.New),
		// TCP without SO_REUSEPORT: with it a node dials from its listen port, so two nodes that
		// dial each other at once get one TCP connection on which both start the security
		// handshake as client, and neither connects. FSC needs no hole punching, which is all
		// that reuseport is for.
		libp2p.Transport(tcp.NewTCPTransport, tcp.DisableReuseport()),
		// Let's prevent our peer from having too many
		// connections by attaching a connection manager.
		libp2p.ConnectionManager(connManager),
		libp2p.BandwidthReporter(newReporter(metrics)),
	}

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		if err := libp2plogging.SetLogLevel("basichost", "debug"); err != nil {
			logger.Errorf("failed setting libp2p log level [%s]", err)
		}
	}

	h, err := libp2p.New(opts...)
	if err != nil {
		logger.Errorf("libp2p: failed to create new host at [%s]: %v", listenAddress, err)
		return nil, err
	}

	libp2pHost := &host{
		Host:    h,
		metrics: metrics,
		peerID:  h.ID().String(),
	}
	logger.Debugf("libp2p: successfully created new host [%s] at [%s]", libp2pHost.ID(), listenAddress)
	return libp2pHost, nil
}

// PeerID returns the local node's libp2p peer identifier as a string. This is
// the value peers observe as the remote peer ID (and SessionInfo.RemotePKID)
// when communicating with this node. It shadows the embedded libp2phost.Host.ID(),
// which returns a peer.ID, to satisfy the host.P2PHost interface.
func (h *host) PeerID() commhost.PeerID {
	return h.peerID
}

// Start registers the handlers for incoming streams on both view protocols. It connects to no
// peer.
func (h *host) Start(newStreamCallback func(stream commhost.P2PStream)) error {
	handler := func(s network.Stream) {
		logger.Debugf("libp2p: received new incoming stream from peer [%s] on [%s]", s.Conn().RemotePeer().String(), s.Protocol())
		uuid := fscutils.GenerateUUID()
		st := &stream{
			Stream: s,
			info: commhost.StreamInfo{
				RemotePeerID:      s.Conn().RemotePeer().String(),
				RemotePeerAddress: s.Conn().RemoteMultiaddr().String(),
				ContextID:         uuid,
				SessionID:         uuid,
			},
		}
		if err := h.openPreamble(context.Background(), st); err != nil {
			return
		}
		newStreamCallback(st)
	}
	h.SetStreamHandler(viewProtocolV2, handler)
	h.SetStreamHandler(viewProtocol, handler)
	logger.Debugf("libp2p: host [%s] started", h.ID())
	return nil
}

func (*host) StreamHash(input commhost.StreamInfo) commhost.StreamHash {
	return streamHash(input)
}

// SharedStreams reports that the streams of this host are shared by all sessions to a peer.
func (*host) SharedStreams() bool {
	return true
}

func (h *host) Close() error {
	logger.Debugf("libp2p: Closing host [%s]...", h.ID())
	err := h.Host.Close()
	logger.Debugf("libp2p: host [%s] closed with error [%v]", h.ID(), err)
	return err
}

func (h *host) NewStream(ctx context.Context, info commhost.StreamInfo) (commhost.P2PStream, error) {
	ID, err := peer.Decode(info.RemotePeerID)
	if err != nil {
		return nil, err
	}

	logger.Debugf("libp2p: attempting to create new outgoing stream to peer [%s]", ID)

	if len(info.RemotePeerAddress) != 0 {
		ps := h.Peerstore()

		addr := info.RemotePeerAddress
		if !strings.HasPrefix(info.RemotePeerAddress, "/ip4/") {
			logger.Debugf("sendTo, reprogram address [%s:%s]", info.RemotePeerID, info.RemotePeerAddress)
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				current := ps.Addrs(ID)
				for _, m := range current {
					logger.Debugf("sendTo, current address [%s:%s]", info.RemotePeerID, m)
				}
			}

			ps.ClearAddrs(ID)
			var err error
			addr, err = utils.AddressToEndpoint(info.RemotePeerAddress)
			if err != nil {
				return nil, errors.WithMessagef(err, "failed to parse endpoint's address [%s]", info.RemotePeerAddress)
			}
		}

		s, err := multiaddr.NewMultiaddr(addr)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get multiaddr for [%s]", info.RemotePeerAddress)
		}
		ps.AddAddr(ID, s, peerstore.RecentlyConnectedAddrTTL)
	}

	// Multistream negotiation picks the first protocol of the list that the peer supports.
	nwStream, err := h.Host.NewStream(ctx, ID, viewProtocolV2, viewProtocol)
	if err != nil {
		logger.Debugf("libp2p: failed to create new stream to peer [%s]: %v", ID, err)
		return nil, errors.Wrapf(err, "failed to create new stream to [%s]", ID)
	}
	logger.Debugf("libp2p: successfully created new outgoing stream to peer [%s] on [%s]", ID, nwStream.Protocol())
	info.RemotePeerID = nwStream.Conn().RemotePeer().String()
	s := &stream{Stream: nwStream, info: info}
	if err := h.openPreamble(ctx, s); err != nil {
		return nil, errors.Wrapf(err, "failed to open stream to [%s]", ID)
	}
	return s, nil
}

// openPreamble exchanges preambles on s if it uses viewProtocolV2, and records the peer's on
// s. On failure s is reset; a preamble that arrived and was refused is also counted.
func (h *host) openPreamble(ctx context.Context, s *stream) error {
	if s.Protocol() != viewProtocolV2 {
		logger.Debugf("libp2p: peer [%s] sent no preamble, it runs an FSC release without version ranges", s.RemotePeerID())
		return nil
	}
	p, refused, err := exchangePreamble(ctx, s.Stream)
	if err != nil {
		if refused {
			h.metrics.RejectedStreams.With(reasonLabel, versionReason).Add(1)
		}
		logger.Warnf("libp2p: refusing stream with peer [%s]: %v", s.RemotePeerID(), err)
		_ = s.Reset()
		return err
	}
	logger.Debugf("libp2p: peer [%s] runs FSC [%s] revision [%s]", s.RemotePeerID(), p.GetFscVersion(), p.GetVcsRevision())
	s.preamble = p
	return nil
}
