/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package libp2p

import (
	"context"
	"encoding/base64"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	utils2 "github.com/hyperledger-labs/fabric-smart-client/pkg/utils"

	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/crypto"
	host3 "github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

var logger = flogging.MustGetLogger("view-sdk.services.comm.libp2p-host")

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

func (h *host) Start(newStreamCallback func(stream host2.P2PStream)) error {
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

func newLibP2PHost(listenAddress host2.PeerIPAddress, priv crypto.PrivKey, metrics *metrics, bootstrap bool, bootstrapNode host2.PeerIPAddress) (*host, error) {
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

	h, err := libp2p.New(opts...)
	if err != nil {
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

	return &host{
		Host:          h,
		finder:        routing.NewRoutingDiscovery(kademliaDHT),
		peers:         make(map[string]peer.AddrInfo),
		bootstrap:     bootstrap,
		bootstrapNode: bootstrapNode,
	}, nil

}

func (h *host) StreamHash(input host2.StreamInfo) host2.StreamHash {
	return streamHash(input)
}

func (h *host) Close() error {

	err := h.Host.Close()
	atomic.StoreInt32(&h.stopFinder, 1)
	return err
}

func (h *host) NewStream(ctx context.Context, info host2.StreamInfo) (host2.P2PStream, error) {
	ID, err := peer.Decode(info.RemotePeerID)
	if err != nil {
		return nil, err
	}

	if len(info.RemotePeerAddress) != 0 && !strings.HasPrefix(info.RemotePeerAddress, "/ip4/") {
		// reprogram the addresses of the peer before opening a new stream, if it is not in the right form yet
		ps := h.Host.Peerstore()
		current := ps.Addrs(ID)

		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("sendTo, reprogram address [%s:%s]", info.RemotePeerID, info.RemotePeerAddress)
			for _, m := range current {
				logger.Debugf("sendTo, current address [%s:%s]", info.RemotePeerID, m)
			}
		}

		ps.ClearAddrs(ID)
		addr, err := utils.AddressToEndpoint(info.RemotePeerAddress)
		if err != nil {
			return nil, errors.WithMessagef(err, "failed to parse endpoint's address [%s]", info.RemotePeerAddress)
		}
		s, err := multiaddr.NewMultiaddr(addr)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get mutliaddr for [%s]", info.RemotePeerAddress)
		}
		ps.AddAddr(ID, s, peerstore.OwnObservedAddrTTL)
	}

	nwStream, err := h.Host.NewStream(ctx, ID, viewProtocol)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create new stream to [%s]", ID)
	}
	return &stream{Stream: nwStream, info: info}, nil
}

func (h *host) startFinder() {
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

func (h *host) start(failAdv bool, newStreamCallback func(stream host2.P2PStream)) error {
	_, err := h.finder.Advertise(context.Background(), rendezVousString)
	if err != nil {
		if failAdv {
			return errors.Wrap(err, "error while announcing")
		}
		logger.Errorf("error while announcing [%s]", err)
	}

	h.Host.SetStreamHandler(viewProtocol, func(s network.Stream) {
		uuid := utils2.GenerateUUID()
		newStreamCallback(
			&stream{
				Stream: s,
				info: host2.StreamInfo{
					RemotePeerID:      base64.StdEncoding.EncodeToString([]byte(s.Conn().RemotePeer())),
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
