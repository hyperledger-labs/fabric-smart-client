/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"io"
	"maps"
	"net"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// Proxy is a round-robin TCP load balancer. It forwards each accepted connection to the
// next backend that accepts a dial, so a backend that is down is skipped. Bytes pass
// through untouched, so TLS ends at the backend.
type Proxy struct {
	listener net.Listener
	backends []string

	mu   sync.Mutex
	next int
	open map[string]int
}

// StartProxy listens on address and forwards to backends until Close.
func StartProxy(address string, backends ...string) (*Proxy, error) {
	if len(backends) == 0 {
		return nil, errors.New("no backends")
	}
	l, err := net.Listen("tcp", address)
	if err != nil {
		return nil, errors.Wrapf(err, "failed listening on [%s]", address)
	}
	p := &Proxy{listener: l, backends: backends, open: map[string]int{}}
	go p.serve()
	return p, nil
}

// Addr returns the address the proxy listens on.
func (p *Proxy) Addr() string { return p.listener.Addr().String() }

// Connections returns the number of open connections per backend address. Backends
// without an open connection are absent.
func (p *Proxy) Connections() map[string]int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return maps.Clone(p.open)
}

// Close stops accepting connections. Open connections stay until either side closes.
func (p *Proxy) Close() error { return p.listener.Close() }

func (p *Proxy) serve() {
	for {
		c, err := p.listener.Accept()
		if err != nil {
			return
		}
		go p.forward(c)
	}
}

func (p *Proxy) forward(c net.Conn) {
	defer func() { _ = c.Close() }()

	p.mu.Lock()
	start := p.next
	p.next = (p.next + 1) % len(p.backends)
	p.mu.Unlock()

	for i := range p.backends {
		backend := p.backends[(start+i)%len(p.backends)]
		b, err := net.DialTimeout("tcp", backend, time.Second)
		if err != nil {
			logger.Debugf("proxy [%s] skips backend [%s]: [%s]", p.Addr(), backend, err)
			continue
		}
		p.count(backend, 1)
		defer p.count(backend, -1)
		defer func() { _ = b.Close() }()

		// The first direction to end closes both connections, which ends the other.
		done := make(chan struct{}, 2)
		go func() { _, _ = io.Copy(b, c); done <- struct{}{} }()
		go func() { _, _ = io.Copy(c, b); done <- struct{}{} }()
		<-done
		return
	}
	logger.Warnf("proxy [%s] found no backend accepting a connection", p.Addr())
}

func (p *Proxy) count(backend string, delta int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.open[backend] += delta; p.open[backend] == 0 {
		delete(p.open, backend)
	}
}
