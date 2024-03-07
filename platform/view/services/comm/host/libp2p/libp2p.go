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

	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

var logger = flogging.MustGetLogger("view-sdk")

const (
	viewProtocol     = "/fsc/view/1.0.0"
	rendezVousString = "fsc"
)

type libP2PStream struct {
	network.Stream
}

type libP2PHost struct {
	host.Host

	finder        *routing.RoutingDiscovery
	finderWg      sync.WaitGroup
	stopFinder    int32
	peersMutex    sync.RWMutex
	peers         map[host2.PeerID]peer.AddrInfo
	bootstrap     bool
	bootstrapNode host2.PeerIPAddress
}

func (s *libP2PStream) RemotePeerID() host2.PeerID {
	return s.Conn().RemotePeer().String()
}
func (s *libP2PStream) RemotePeerAddress() host2.PeerIPAddress {
	return s.Conn().RemoteMultiaddr().String()
}

func (h *libP2PHost) Wait() {
	h.finderWg.Wait()
}

func (h *libP2PHost) Lookup(peerID string) ([]host2.PeerIPAddress, bool) {
	h.peersMutex.RLock()
	defer h.peersMutex.RUnlock()

	peer, in := h.peers[peerID]
	if !in {
		return []string{}, false
	}
	addrs := make([]string, len(peer.Addrs))
	for i, addr := range peer.Addrs {
		addrs[i] = addr.String()
	}
	return addrs, in
}

func (h *libP2PHost) Start(newStreamCallback func(stream host2.P2PStream)) error {
	if h.bootstrap {
		h.Peerstore().AddAddrs(h.ID(), h.Addrs(), time.Hour)
		if err := h.start(false, newStreamCallback); err != nil {
			return err
		}
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

	err = h.Connect(context.Background(), *peerinfo)
	if err != nil {
		return err
	}
	if err := h.start(false, newStreamCallback); err != nil {
		return err
	}

	return nil
}

func newLibP2PHost(listenAddress host2.PeerIPAddress, priv crypto.PrivKey, metrics *Metrics, bootstrap bool, bootstrapNode host2.PeerIPAddress) (*libP2PHost, error) {
	addr, err := multiaddr.NewMultiaddr(listenAddress)
	if err != nil {
		return nil, err
	}

	opts := []libp2p.Option{
		libp2p.ListenAddrs(addr),
		libp2p.Identity(priv),
		libp2p.ForceReachabilityPublic(),
		libp2p.BandwidthReporter(newReporter(metrics)),
	}

	host, err := libp2p.New(opts...)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	kademliaDHT, err := dht.New(ctx, host)
	if err != nil {
		return nil, err
	}

	err = kademliaDHT.Bootstrap(ctx)
	if err != nil {
		return nil, err
	}

	return &libP2PHost{
		Host:          host,
		finder:        routing.NewRoutingDiscovery(kademliaDHT),
		peers:         make(map[string]peer.AddrInfo),
		bootstrap:     bootstrap,
		bootstrapNode: bootstrapNode,
	}, nil

}

func (h *libP2PHost) Close() error {

	err := h.Host.Close()
	atomic.StoreInt32(&h.stopFinder, 1)
	return err
}

func (h *libP2PHost) NewStream(ctx context.Context, address host2.PeerIPAddress, peerID host2.PeerID) (host2.P2PStream, error) {
	ID, err := peer.Decode(peerID)
	if err != nil {
		return nil, err
	}

	if len(address) != 0 && strings.HasPrefix(address, "/ip4/") {
		// reprogram the addresses of the peer before opening a new stream, if it is not in the right form yet
		ps := h.Host.Peerstore()
		current := ps.Addrs(ID)

		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("sendTo, reprogram address [%s:%s]", peerID, address)
			for _, m := range current {
				logger.Debugf("sendTo, current address [%s:%s]", peerID, m.String())
			}
		}

		ps.ClearAddrs(ID)
		addr, err := utils.AddressToEndpoint(address)
		if err != nil {
			return nil, errors.WithMessagef(err, "failed to parse endpoint's address [%s]", address)
		}
		s, err := multiaddr.NewMultiaddr(addr)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get mutliaddr for [%s]", address)
		}
		ps.AddAddr(ID, s, peerstore.OwnObservedAddrTTL)
	}

	nwStream, err := h.Host.NewStream(ctx, ID, viewProtocol)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create new stream to [%s]", ID)
	}

	return &libP2PStream{Stream: nwStream}, nil
}

func (h *libP2PHost) startFinder() {
	for {
		peerChan, err := h.finder.FindPeers(context.Background(), rendezVousString)
		if err != nil {
			logger.Errorf("got error from peer finder: %s\n", err.Error())
			goto sleep
		}

		for peer := range peerChan {
			if peer.ID == h.Host.ID() {
				continue
			}

			h.peersMutex.Lock()
			if _, in := h.peers[peer.ID.String()]; !in {
				logger.Debugf("found peer [%v]", peer)
				h.peers[peer.ID.String()] = peer
			}
			h.peersMutex.Unlock()
		}

	sleep:
		for i := 0; i < 4; i++ {
			if atomic.LoadInt32(&h.stopFinder) != 0 {
				h.finderWg.Done()
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func (h *libP2PHost) start(failAdv bool, newStreamCallback func(stream host2.P2PStream)) error {
	_, err := h.finder.Advertise(context.Background(), rendezVousString)
	if err != nil {
		if failAdv {
			return errors.Wrap(err, "error while announcing")
		}
		logger.Errorf("error while announcing [%s]", err)
	}

	h.Host.SetStreamHandler(viewProtocol, func(stream network.Stream) {
		newStreamCallback(&libP2PStream{Stream: stream})
	})

	h.finderWg.Add(1)
	go h.startFinder()

	return nil
}
