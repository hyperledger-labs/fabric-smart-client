/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"fmt"
	"io/ioutil"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/multiformats/go-multiaddr"
)

func newHost(ListenAddress string, keyDispenser PrivateKeyDispenser) (*P2PNode, error) {
	priv, err := keyDispenser.PrivateKey()
	if err != nil {
		return nil, err
	}

	addr, err := multiaddr.NewMultiaddr(ListenAddress)
	if err != nil {
		return nil, err
	}

	opts := []libp2p.Option{
		libp2p.ListenAddrs(addr),
		libp2p.Identity(priv),
		libp2p.ForceReachabilityPublic(),
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

	node := &P2PNode{
		host:             host,
		dht:              kademliaDHT,
		finder:           routing.NewRoutingDiscovery(kademliaDHT),
		peers:            make(map[string]peer.AddrInfo),
		incomingMessages: make(chan *messageWithStream),
		streams:          make(map[peer.ID][]*streamHandler),
		sessions:         make(map[string]*NetworkStreamSession),
		isStopping:       false,
	}

	return node, err
}

type PrivateKeyFromCryptoKey struct {
	Key crypto.PrivKey
}

func (p *PrivateKeyFromCryptoKey) PrivateKey() (crypto.PrivKey, error) {
	return p.Key, nil
}

type PrivateKeyFromFile struct {
	PrivateKeyFile string
}

func (p *PrivateKeyFromFile) PrivateKey() (crypto.PrivKey, error) {
	privBytes, err := ioutil.ReadFile(p.PrivateKeyFile)
	if err != nil {
		return nil, err
	}

	return crypto.UnmarshalECDSAPrivateKey(privBytes)
}

func NewBootstrapNode(ListenAddress string, keyDispenser PrivateKeyDispenser) (*P2PNode, error) {
	node, err := newHost(ListenAddress, keyDispenser)
	if err != nil {
		return nil, err
	}

	node.host.Peerstore().AddAddrs(node.host.ID(), node.host.Addrs(), time.Hour)

	node.start()

	return node, nil
}

func NewNode(ListenAddress, BootstrapNode string, keyDispenser PrivateKeyDispenser) (*P2PNode, error) {
	node, err := newHost(ListenAddress, keyDispenser)
	if err != nil {
		return nil, err
	}

	addr, err := multiaddr.NewMultiaddr(BootstrapNode)
	if err != nil {
		return nil, err
	}

	peerinfo, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		return nil, err
	}

	err = node.host.Connect(context.Background(), *peerinfo)
	if err != nil {
		return nil, err
	}

	node.start()

	return node, nil
}

func (p *P2PNode) startFinder() {
	for {
		peerChan, err := p.finder.FindPeers(context.Background(), rendezVousString)
		if err != nil {
			fmt.Printf("got error from peer finder: %s\n", err.Error())
			goto sleep
		}

		for peer := range peerChan {
			if peer.ID == p.host.ID() {
				continue
			}

			p.peersMutex.Lock()
			if _, in := p.peers[peer.ID.String()]; !in {
				// fmt.Print("Found peer:", peer)
				p.peers[peer.ID.String()] = peer
			}
			p.peersMutex.Unlock()
		}

	sleep:
		for i := 0; i < 4; i++ {
			if atomic.LoadInt32(&p.stopFinder) != 0 {
				p.finderWg.Done()
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func (p *P2PNode) start() {
	_, err := p.finder.Advertise(context.Background(), rendezVousString)
	if err != nil {
		logger.Debugf("error while announcing: %s", err)
	}

	p.host.SetStreamHandler(protocol.ID(viewProtocol), p.handleStream())

	p.finderWg.Add(1)
	go p.startFinder()
}
