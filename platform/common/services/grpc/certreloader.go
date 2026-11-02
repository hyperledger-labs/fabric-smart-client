/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc

import (
	"crypto/tls"
	"crypto/x509"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
)

// CertReloader serves a listener's TLS certificate through [tls.Config.GetCertificate] and,
// when a reload interval is configured, re-reads the certificate and key files every
// interval, so a rotated certificate is served without restarting the node.
//
// A reload whose files do not parse, or whose certificate and key do not match, is refused:
// the previous pair keeps being served and a warning is logged. A new pair applies to new
// handshakes only; established connections keep the certificate they negotiated.
//
// It is safe for concurrent use. Start and Stop accept a nil receiver, so a listener without
// TLS can call them unconditionally.
type CertReloader struct {
	certFile, keyFile string
	interval          time.Duration

	cert atomic.Pointer[tls.Certificate]

	// mu guards gauge.
	mu    sync.Mutex
	gauge metrics.Gauge

	// stopped is closed by Stop. Start and Stop run at most once each, so a Stop that wins a
	// race with Start still ends the reload goroutine.
	stopped     chan struct{}
	start, stop func()
}

// NewCertReloader returns a reloader serving the keypair in so.Certificate and so.Key, which
// re-reads so.CertFile and so.KeyFile every so.ReloadInterval once started. It returns an
// error when the keypair does not parse or its halves do not match.
func NewCertReloader(so SecureOptions) (*CertReloader, error) {
	cert, err := tls.X509KeyPair(so.Certificate, so.Key)
	if err != nil {
		return nil, errors.Wrap(err, "failed loading TLS key pair")
	}
	r := &CertReloader{
		certFile: so.CertFile, keyFile: so.KeyFile, interval: so.ReloadInterval,
		stopped: make(chan struct{}),
	}
	r.start = sync.OnceFunc(func() { go r.run() })
	r.stop = sync.OnceFunc(func() { close(r.stopped) })
	if err := r.set(cert); err != nil {
		return nil, err
	}
	return r, nil
}

// GetCertificate returns the current certificate. It has the signature of
// [tls.Config.GetCertificate].
func (r *CertReloader) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return r.cert.Load(), nil
}

// NotAfter returns the expiry of the current certificate.
func (r *CertReloader) NotAfter() time.Time {
	return r.cert.Load().Leaf.NotAfter
}

// ReportExpiry sets g to the current certificate's expiry in Unix seconds, now and after
// every successful reload.
func (r *CertReloader) ReportExpiry(g metrics.Gauge) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gauge = g
	g.Set(float64(r.NotAfter().Unix()))
}

// set serves cert from the next handshake on. It parses the leaf when cert lacks one, so
// NotAfter always has an expiry to report.
func (r *CertReloader) set(cert tls.Certificate) error {
	if cert.Leaf == nil {
		if len(cert.Certificate) == 0 {
			return errors.New("TLS certificate has no certificate data")
		}
		leaf, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return errors.Wrap(err, "failed parsing TLS certificate")
		}
		cert.Leaf = leaf
	}
	r.cert.Store(&cert)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.gauge != nil {
		r.gauge.Set(float64(r.NotAfter().Unix()))
	}
	return nil
}

// Reload re-reads the certificate and key files and serves the pair from the next handshake
// on. On error the current pair stays in place.
func (r *CertReloader) Reload() error {
	if r.certFile == "" || r.keyFile == "" {
		return errors.New("no certificate and key files to reload from")
	}
	cert, err := tls.LoadX509KeyPair(r.certFile, r.keyFile)
	if err != nil {
		return errors.Wrapf(err, "failed reloading TLS key pair [%s], [%s]", r.certFile, r.keyFile)
	}
	return r.set(cert)
}

// Start reloads the files every interval until Stop. It does nothing when the interval is not
// positive, on a second call, or after Stop.
func (r *CertReloader) Start() {
	if r != nil && r.interval > 0 {
		r.start()
	}
}

func (r *CertReloader) run() {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-r.stopped:
			return
		case <-ticker.C:
			if err := r.Reload(); err != nil {
				commLogger.Warnf("keeping the current TLS certificate: %v", err)
				continue
			}
			commLogger.Debugf("reloaded TLS certificate [%s], expires [%s]", r.certFile, r.NotAfter())
		}
	}
}

// Stop ends the reloads Start began, and keeps a later Start from beginning them. A reload
// already in progress completes.
func (r *CertReloader) Stop() {
	if r != nil {
		r.stop()
	}
}
