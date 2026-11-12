/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"io"
	"maps"
	"net"
	"slices"
	"sync"
	"sync/atomic"
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
	// forwarding holds every connection being forwarded, keyed by its client side, so
	// CloseConnections can close it and wait until it is no longer forwarded.
	forwarding map[net.Conn]*forwarded
	// healed is closed by Heal; it is nil while the proxy is not partitioned.
	healed chan struct{}
	// latency delays each forwarded chunk, see SetLatency.
	latency time.Duration
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
	p := &Proxy{listener: l, backends: backends, open: map[string]int{}, forwarding: map[net.Conn]*forwarded{}}
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

// Close stops accepting connections and ends a partition. Open connections stay until
// either side closes.
func (p *Proxy) Close() error {
	p.Heal()
	return p.listener.Close()
}

// CloseConnections closes both sides of every connection the proxy forwards, as a load
// balancer does when it drops its connections on an idle timeout or a restart, and keeps
// accepting new ones. Each side reads the end of the stream at once, also while the proxy is
// partitioned. It returns once none of the closed connections is forwarded any more, so
// Connections no longer counts them. Connections accepted meanwhile are forwarded as usual,
// and held while the proxy is partitioned.
func (p *Proxy) CloseConnections() {
	p.mu.Lock()
	closing := slices.Collect(maps.Values(p.forwarding))
	p.mu.Unlock()
	for _, f := range closing {
		f.closing.Store(true)
		_ = f.client.Close()
		_ = f.backend.Close()
	}
	for _, f := range closing {
		<-f.done
	}
}

// forwarded is a connection the proxy forwards to a backend; done is closed once it is no
// longer forwarded. closing is set once CloseConnections closes it, so its pipes end without
// waiting for a Heal.
type forwarded struct {
	client, backend net.Conn
	done            chan struct{}
	closing         atomic.Bool
}

// Partition stops forwarding bytes in both directions until Heal, without closing any
// connection: neither side sees a reset or an end of stream, so only a keepalive or a
// timeout of its own detects the partition. Bytes read during the partition, including
// those of connections accepted during it, are held, not dropped, so after Heal every
// connection resumes where it stopped; a chunk already read when Partition is called may
// still be delivered. A connection that one side closes during the partition is closed on
// the other side only after Heal; one that CloseConnections closes ends at once. Partition
// on a partitioned proxy does nothing.
func (p *Proxy) Partition() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.healed == nil {
		p.healed = make(chan struct{})
	}
}

// Heal ends a partition started by Partition. Heal on a proxy that is not partitioned
// does nothing.
func (p *Proxy) Heal() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.healed != nil {
		close(p.healed)
		p.healed = nil
	}
}

// SetLatency delays every chunk the proxy forwards, in both directions, by d, until it is
// set again; zero forwards without delay. It applies to open connections from their next
// chunk on. Chunks of one direction stay in order, so a connection's throughput drops to
// one chunk per d.
func (p *Proxy) SetLatency(d time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.latency = d
}

// waitHealed blocks while the proxy is partitioned, then waits out the latency.
func (p *Proxy) waitHealed() {
	p.mu.Lock()
	healed, latency := p.healed, p.latency
	p.mu.Unlock()
	if healed != nil {
		<-healed
	}
	time.Sleep(latency)
}

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
		f := p.track(c, b, backend)
		defer p.untrack(f, backend)
		defer func() { _ = b.Close() }()

		// The first direction to end closes both connections, which ends the other.
		done := make(chan struct{}, 2)
		go func() { p.pipe(b, c, f); done <- struct{}{} }()
		go func() { p.pipe(c, b, f); done <- struct{}{} }()
		<-done
		return
	}
	logger.Warnf("proxy [%s] found no backend accepting a connection", p.Addr())
}

// pipe copies src to dst, the two sides of f, until src ends or dst fails. It holds each
// chunk, and the end of src, while the proxy is partitioned, and delays each by the latency;
// once CloseConnections closes f, it returns without either.
func (p *Proxy) pipe(dst io.Writer, src io.Reader, f *forwarded) {
	buf := make([]byte, 32*1024)
	for {
		n, err := src.Read(buf)
		if f.closing.Load() {
			return
		}
		p.waitHealed()
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// track records that c is forwarded to b at backend.
func (p *Proxy) track(c, b net.Conn, backend string) *forwarded {
	f := &forwarded{client: c, backend: b, done: make(chan struct{})}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.forwarding[c] = f
	p.open[backend]++
	return f
}

// untrack records that the forwarding of f to backend has ended.
func (p *Proxy) untrack(f *forwarded, backend string) {
	p.mu.Lock()
	delete(p.forwarding, f.client)
	if p.open[backend]--; p.open[backend] == 0 {
		delete(p.open, backend)
	}
	p.mu.Unlock()
	close(f.done)
}
