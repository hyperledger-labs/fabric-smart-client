/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc_test

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	commongrpc "github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc/tlsgen"
)

// reloaderFixture writes a server keypair to a temp dir and returns a reloader over it plus a
// func that overwrites the files with a fresh keypair from the same CA.
func reloaderFixture(t *testing.T, interval time.Duration) (*commongrpc.CertReloader, string, func() *tlsgen.CertKeyPair) {
	t.Helper()
	dir := t.TempDir()
	ca, err := tlsgen.NewCA()
	require.NoError(t, err)
	certFile, keyFile := filepath.Join(dir, "s.crt"), filepath.Join(dir, "s.key")
	rotate := func() *tlsgen.CertKeyPair {
		kp, err := ca.NewServerCertKeyPair("127.0.0.1")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(certFile, kp.Cert, 0o600))
		require.NoError(t, os.WriteFile(keyFile, kp.Key, 0o600))
		return kp
	}
	kp := rotate()
	r, err := commongrpc.NewCertReloader(commongrpc.SecureOptions{
		Certificate: kp.Cert, Key: kp.Key, CertFile: certFile, KeyFile: keyFile, ReloadInterval: interval,
	})
	require.NoError(t, err)
	return r, dir, rotate
}

func served(t *testing.T, r *commongrpc.CertReloader) []byte {
	t.Helper()
	c, err := r.GetCertificate(&tls.ClientHelloInfo{})
	require.NoError(t, err)
	return c.Certificate[0]
}

func der(t *testing.T, certPEM []byte) []byte {
	t.Helper()
	b, _ := pem.Decode(certPEM)
	require.NotNil(t, b)
	return b.Bytes
}

func TestCertReloaderServesRenewedCertificate(t *testing.T) {
	t.Parallel()
	r, _, rotate := reloaderFixture(t, 0)
	kp := rotate()
	require.NotEqual(t, der(t, kp.Cert), served(t, r), "nothing is re-read before Reload")

	require.NoError(t, r.Reload())
	require.Equal(t, der(t, kp.Cert), served(t, r))
}

func TestCertReloaderRefusesBrokenPair(t *testing.T) {
	t.Parallel()
	r, dir, rotate := reloaderFixture(t, 0)
	before := served(t, r)

	// A renewed certificate next to the old key: the halves no longer match.
	oldKey, err := os.ReadFile(filepath.Join(dir, "s.key"))
	require.NoError(t, err)
	rotate()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "s.key"), oldKey, 0o600))
	require.Error(t, r.Reload(), "mismatched certificate and key")
	require.Equal(t, before, served(t, r))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "s.crt"), []byte("garbage"), 0o600))
	require.Error(t, r.Reload(), "corrupt certificate")
	require.Equal(t, before, served(t, r))
}

func TestCertReloaderZeroIntervalNeverRereads(t *testing.T) {
	t.Parallel()
	r, _, rotate := reloaderFixture(t, 0)
	before := served(t, r)
	r.Start()
	t.Cleanup(r.Stop)
	rotate()
	r.Stop()
	require.Equal(t, before, served(t, r))
}

// The periodic path, under -race with concurrent handshakes.
func TestCertReloaderPeriodicReload(t *testing.T) {
	t.Parallel()
	r, _, rotate := reloaderFixture(t, 10*time.Millisecond)
	r.Start()
	t.Cleanup(r.Stop)

	var wg sync.WaitGroup
	done := make(chan struct{})
	for range 4 {
		wg.Go(func() {
			for {
				select {
				case <-done:
					return
				default:
					_, _ = r.GetCertificate(nil)
				}
			}
		})
	}
	kp := rotate()
	require.Eventually(t, func() bool {
		c, _ := r.GetCertificate(nil)
		return string(c.Certificate[0]) == string(der(t, kp.Cert))
	}, 5*time.Second, 10*time.Millisecond)
	close(done)
	wg.Wait()
	r.Stop()
	r.Stop() // idempotent
}

// A Stop that runs before Start, as when a node stops while its listener is still starting,
// keeps Start from launching a reload goroutine nobody stops.
func TestCertReloaderStopBeforeStart(t *testing.T) {
	t.Parallel()
	r, _, rotate := reloaderFixture(t, time.Millisecond)
	before := served(t, r)
	r.Stop()
	r.Start()
	rotate()
	time.Sleep(20 * time.Millisecond) // many intervals
	require.Equal(t, before, served(t, r))
}

func TestCertReloaderReportExpiry(t *testing.T) {
	t.Parallel()
	r, _, rotate := reloaderFixture(t, 0)
	g := &mock.Gauge{}
	r.ReportExpiry(g)
	require.Equal(t, 1, g.SetCallCount())
	require.Equal(t, r.NotAfter().Unix(), int64(g.SetArgsForCall(0)))

	kp := rotate()
	require.NoError(t, r.Reload())
	leaf, err := x509.ParseCertificate(der(t, kp.Cert))
	require.NoError(t, err)
	require.Equal(t, 2, g.SetCallCount())
	require.Equal(t, leaf.NotAfter.Unix(), int64(g.SetArgsForCall(1)))
}

func TestNewCertReloaderRejectsMismatchedPair(t *testing.T) {
	t.Parallel()
	ca, err := tlsgen.NewCA()
	require.NoError(t, err)
	a, err := ca.NewServerCertKeyPair("127.0.0.1")
	require.NoError(t, err)
	b, err := ca.NewServerCertKeyPair("127.0.0.1")
	require.NoError(t, err)
	_, err = commongrpc.NewCertReloader(commongrpc.SecureOptions{Certificate: a.Cert, Key: b.Key})
	require.Error(t, err)
}
