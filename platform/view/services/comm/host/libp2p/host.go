/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package libp2p

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	utils2 "github.com/hyperledger-labs/fabric-smart-client/pkg/utils"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/utils"
	libp2plogging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/crypto"
	host3 "github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2ptls "github.com/libp2p/go-libp2p/p2p/security/tls"
	"github.com/multiformats/go-multiaddr"
	"go.uber.org/zap/zapcore"
)

var logger = logging.MustGetLogger()

const (
	viewProtocol     = "/fsc/view/1.0.0"
	rendezVousString = "fsc"
)

type host struct {
	host3.Host

	finder        *routing.RoutingDiscovery
	finderWg      sync.WaitGroup
	stopFinder    int32
	peersMutex    sync.RWMutex
	peers         map[host2.PeerID]peer.AddrInfo
	bootstrap     bool
	bootstrapNode host2.PeerIPAddress
}

func (h *host) Wait() {
	h.finderWg.Wait()
}

func (h *host) Lookup(peerID string) ([]host2.PeerIPAddress, bool) {
	logger.Debugf("libp2p: Looking up addresses for peer [%s]...", peerID)
	h.peersMutex.RLock()
	defer h.peersMutex.RUnlock()

	peer, in := h.peers[peerID]
	if !in {
		logger.Debugf("libp2p: Found NO addresses for peer [%s]", peerID)
		return []string{}, false
	}
	addrs := make([]string, len(peer.Addrs))
	for i, addr := range peer.Addrs {
		addrs[i] = addr.String()
	}
	logger.Debugf("libp2p: Found addresses for peer [%s]: %v", peerID, addrs)
	return addrs, in
}

func (h *host) Start(newStreamCallback func(stream host2.P2PStream)) error {
	logger.Debugf("libp2p: Starting host [%s] (bootstrap: %v, bootstrapNode: [%s])...", h.ID(), h.bootstrap, h.bootstrapNode)
	if h.bootstrap {
		h.Peerstore().AddAddrs(h.ID(), h.Addrs(), time.Hour)
		if err := h.start(false, newStreamCallback); err != nil {
			logger.Errorf("libp2p: failed to start bootstrap host [%s]: %v", h.ID(), err)
			return err
		}
		logger.Debugf("libp2p: bootstrap host [%s] started successfully", h.ID())
		return nil
	}

	addr, err := multiaddr.NewMultiaddr(h.bootstrapNode)
	if err != nil {
		return err
	}

	peerinfo, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		return err
	}

	logger.Debugf("libp2p: connecting to bootstrap node [%s]...", h.bootstrapNode)
	err = h.Connect(context.Background(), *peerinfo)
	if err != nil {
		logger.Errorf("libp2p: failed to connect host [%s] to bootstrap node [%s]: %v", h.ID(), h.bootstrapNode, err)
		return err
	}
	logger.Debugf("libp2p: host [%s] connected to bootstrap node [%s]", h.ID(), h.bootstrapNode)
	if err := h.start(false, newStreamCallback); err != nil {
		logger.Errorf("libp2p: failed to start host [%s]: %v", h.ID(), err)
		return err
	}
	logger.Debugf("libp2p: host [%s] started successfully", h.ID())
	return nil
}

func newLibP2PHost(listenAddress host2.PeerIPAddress, priv crypto.PrivKey, metrics *metrics, bootstrap bool, bootstrapNode host2.PeerIPAddress) (*host, error) {
	logger.Debugf("libp2p: Creating new host at [%s], bootstrap: %v, bootstrapNode: [%s]...", listenAddress, bootstrap, bootstrapNode)
	addr, err := multiaddr.NewMultiaddr(listenAddress)
	if err != nil {
		return nil, err
	}

	connManager, err := connmgr.NewConnManager(100, 400, connmgr.WithGracePeriod(time.Minute))
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
		// support any other default transports (TCP)
		libp2p.DefaultTransports,
		// Let's prevent our peer from having too many
		// connections by attaching a connection manager.
		libp2p.ConnectionManager(connManager), libp2p.ForceReachabilityPublic(),
		libp2p.BandwidthReporter(newReporter(metrics)),
	}

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		libp2plogging.SetDebugLogging()
	}

	h, err := libp2p.New(opts...)
	if err != nil {
		logger.Errorf("libp2p: failed to create new host at [%s]: %v", listenAddress, err)
		return nil, err
	}

	ctx := context.Background()
	kademliaDHT, err := dht.New(ctx, h)
	if err != nil {
		return nil, err
	}

	err = kademliaDHT.Bootstrap(ctx)
	if err != nil {
		return nil, err
	}

	libp2pHost := &host{
		Host:          h,
		finder:        routing.NewRoutingDiscovery(kademliaDHT),
		peers:         make(map[string]peer.AddrInfo),
		bootstrap:     bootstrap,
		bootstrapNode: bootstrapNode,
	}
	logger.Debugf("libp2p: successfully created new host [%s] at [%s]", libp2pHost.ID(), listenAddress)
	return libp2pHost, nil
}

func (h *host) StreamHash(input host2.StreamInfo) host2.StreamHash {
	return streamHash(input)
}

func (h *host) Close() error {
	logger.Debugf("libp2p: Closing host [%s]...", h.ID())
	err := h.Host.Close()
	atomic.StoreInt32(&h.stopFinder, 1)
	logger.Debugf("libp2p: host [%s] closed with error [%v]", h.ID(), err)
	return err
}

func (h *host) NewStream(ctx context.Context, info host2.StreamInfo) (host2.P2PStream, error) {
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
			return nil, errors.Wrapf(err, "failed to get mutliaddr for [%s]", info.RemotePeerAddress)
		}
		ps.AddAddr(ID, s, peerstore.RecentlyConnectedAddrTTL)
	}

	nwStream, err := h.Host.NewStream(ctx, ID, viewProtocol)
	if err != nil {
		logger.Debugf("libp2p: failed to create new stream to peer [%s]: %v", ID, err)
		return nil, errors.Wrapf(err, "failed to create new stream to [%s]", ID)
	}
	logger.Debugf("libp2p: successfully created new outgoing stream to peer [%s]", ID)
	info.RemotePeerID = nwStream.Conn().RemotePeer().String()
	return &stream{
		Stream: nwStream,
		info:   info,
	}, nil
}

func (h *host) startFinder() {
	logger.Debugf("libp2p: starting peer finder for host [%s]...", h.ID())
	for {
		peerChan, err := h.finder.FindPeers(context.Background(), rendezVousString)
		if err != nil {
			logger.Errorf("got error from peer finder: %s\n", err.Error())
			goto sleep
		}

		for peer := range peerChan {
			if peer.ID == h.ID() {
				continue
			}

			h.peersMutex.Lock()
			if _, in := h.peers[peer.ID.String()]; !in {
				logger.Debugf("libp2p: Found new peer [%s] at %v", peer.ID, peer.Addrs)
				h.peers[peer.ID.String()] = peer
			}
			h.peersMutex.Unlock()
		}

	sleep:
		for i := 0; i < 4; i++ {
			if atomic.LoadInt32(&h.stopFinder) != 0 {
				logger.Debugf("libp2p: stopping peer finder for host [%s]", h.ID())
				h.finderWg.Done()
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func (h *host) start(failAdv bool, newStreamCallback func(stream host2.P2PStream)) error {
	logger.Debugf("libp2p: Advertising rendez-vous [%s]...", rendezVousString)
	_, err := h.finder.Advertise(context.Background(), rendezVousString)
	if err != nil {
		if failAdv {
			return errors.Wrap(err, "error while announcing")
		}
		logger.Warnf("error while announcing [%s]", err)
	}

	h.SetStreamHandler(viewProtocol, func(s network.Stream) {
		logger.Debugf("libp2p: received new incoming stream from peer [%s]", s.Conn().RemotePeer().String())
		uuid := utils2.GenerateUUID()
		newStreamCallback(
			&stream{
				Stream: s,
				info: host2.StreamInfo{
					RemotePeerID:      s.Conn().RemotePeer().String(),
					RemotePeerAddress: s.Conn().RemoteMultiaddr().String(),
					ContextID:         uuid,
					SessionID:         uuid,
				},
			},
		)
	})

	h.finderWg.Add(1)
	go h.startFinder()

	return nil
}
