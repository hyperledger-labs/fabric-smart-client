/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	config2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
)

type extraCAs [][]byte

func (e extraCAs) ExtraCAs() [][]byte { return e }

// Both builders apply the same rules to their keypair, and trust only the configured pool.
func TestTLSConfigBuildersRejectBadMaterial(t *testing.T) {
	t.Parallel()
	cert, key, err := GenerateTestCert("node")
	require.NoError(t, err)

	// GetNewHost relies on (nil, nil) to refuse a host without TLS.
	cfg, err := newClientTLSConfig(nil, grpc.SecureOptions{})
	require.NoError(t, err)
	require.Nil(t, cfg)
	cfg, err = newServerTLSConfig(nil, grpc.SecureOptions{}, nil)
	require.NoError(t, err)
	require.Nil(t, cfg)

	for name, build := range map[string]func(*x509.CertPool, grpc.SecureOptions) (*tls.Config, error){
		"client": newClientTLSConfig,
		"server": func(pool *x509.CertPool, opts grpc.SecureOptions) (*tls.Config, error) {
			return newServerTLSConfig(pool, opts, nil)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := build(x509.NewCertPool(), grpc.SecureOptions{Certificate: cert})
			require.ErrorContains(t, err, "both "+name+" key and cert must be set")

			_, err = build(x509.NewCertPool(), grpc.SecureOptions{Certificate: []byte("junk"), Key: []byte("junk")})
			require.ErrorContains(t, err, "failed to load "+name+" x509 certificates")

			base := x509.NewCertPool()
			cfg, err := build(base, grpc.SecureOptions{Certificate: cert, Key: key, RequireClientCert: true})
			require.NoError(t, err)
			if name == "server" {
				require.Same(t, base, cfg.ClientCAs)
			} else {
				require.Same(t, base, cfg.RootCAs)
			}
		})
	}
}

func TestWithExtraCAs(t *testing.T) {
	t.Parallel()
	otherCA, _, err := GenerateTestCert("other")
	require.NoError(t, err)
	base := x509.NewCertPool()

	// Without extra CAs the configured pool is used as is.
	require.Same(t, base, withExtraCAs(base, nil))

	// A valid extra CA lands in a clone; the configured pool is never mutated.
	withOther := withExtraCAs(base, [][]byte{otherCA})
	require.False(t, base.Equal(withOther))
	require.True(t, base.Equal(x509.NewCertPool()))

	// An extra CA that is not a PEM certificate is skipped, not fatal, and does not keep the
	// valid ones out.
	require.True(t, withOther.Equal(withExtraCAs(base, [][]byte{[]byte("not a pem"), otherCA})))
}

func TestClientTLSConfigPresentsConfiguredCertificate(t *testing.T) {
	t.Parallel()
	certPEM, key, err := GenerateTestCert("node")
	require.NoError(t, err)
	cfg, err := newClientTLSConfig(x509.NewCertPool(), grpc.SecureOptions{Certificate: certPEM, Key: key})
	require.NoError(t, err)
	require.Equal(t, uint16(tls.VersionTLS13), cfg.MinVersion)
	require.Equal(t, uint16(tls.VersionTLS13), cfg.MaxVersion)

	dn, err := asn1.Marshal(pkix.Name{CommonName: "some-ca"}.ToRDNSequence())
	require.NoError(t, err)
	want, err := tls.X509KeyPair(certPEM, key)
	require.NoError(t, err)

	// A DN that cannot be parsed is only logged; the certificate is still returned.
	got, err := cfg.GetClientCertificate(&tls.CertificateRequestInfo{AcceptableCAs: [][]byte{dn, []byte("garbage")}})
	require.NoError(t, err)
	require.Equal(t, want.Certificate, got.Certificate)
}

func TestServerTLSConfigClientAuth(t *testing.T) {
	t.Parallel()
	cert, key, err := GenerateTestCert("node")
	require.NoError(t, err)
	otherCA, _, err := GenerateTestCert("other")
	require.NoError(t, err)
	opts := func(required bool) grpc.SecureOptions {
		return grpc.SecureOptions{Certificate: cert, Key: key, RequireClientCert: required}
	}

	t.Run("not required", func(t *testing.T) {
		t.Parallel()
		cfg, err := newServerTLSConfig(x509.NewCertPool(), opts(false), extraCAs{})
		require.NoError(t, err)
		require.Equal(t, tls.NoClientCert, cfg.ClientAuth)
		require.Nil(t, cfg.GetConfigForClient)
	})

	t.Run("required", func(t *testing.T) {
		t.Parallel()
		base := x509.NewCertPool()
		provider := extraCAs{}
		cfg, err := newServerTLSConfig(base, opts(true), &provider)
		require.NoError(t, err)
		require.Equal(t, tls.RequireAndVerifyClientCert, cfg.ClientAuth)

		// The extra CAs are re-read on every handshake.
		perHandshake, err := cfg.GetConfigForClient(&tls.ClientHelloInfo{})
		require.NoError(t, err)
		require.Same(t, cfg, perHandshake)

		provider = extraCAs{otherCA}
		perHandshake, err = cfg.GetConfigForClient(&tls.ClientHelloInfo{})
		require.NoError(t, err)
		require.NotSame(t, cfg, perHandshake)
		require.False(t, base.Equal(perHandshake.ClientCAs))
		require.True(t, base.Equal(x509.NewCertPool()), "the configured pool must not be mutated")

		// A resolver ID that is not a certificate does not fail the handshake or drop the
		// valid extra CAs.
		withOther := perHandshake.ClientCAs
		provider = extraCAs{[]byte("not a pem"), otherCA}
		perHandshake, err = cfg.GetConfigForClient(&tls.ClientHelloInfo{})
		require.NoError(t, err)
		require.True(t, withOther.Equal(perHandshake.ClientCAs))
	})

	t.Run("anchor known at startup is withdrawn", func(t *testing.T) {
		t.Parallel()
		base := x509.NewCertPool()
		provider := extraCAs{otherCA}
		cfg, err := newServerTLSConfig(base, opts(true), &provider)
		require.NoError(t, err)

		provider = extraCAs{}
		perHandshake, err := cfg.GetConfigForClient(&tls.ClientHelloInfo{})
		require.NoError(t, err)
		require.Same(t, base, perHandshake.ClientCAs)
	})
}

func TestNewConfigParsesWebsocketOptions(t *testing.T) {
	t.Parallel()
	withOpts := func(listenAddress, opts string) string {
		return strings.Replace(
			strings.Replace(p2pCore, "/ip4/127.0.0.1/tcp/9000", listenAddress, 1),
			"      websocket:\n", "      websocket:\n"+opts, 1)
	}
	load := func(t *testing.T, body string) (*config, error) {
		t.Helper()
		dir, _, _ := writeNodeDir(t, body)
		p, err := config2.NewProvider(dir)
		require.NoError(t, err)
		return NewConfig(p)
	}

	t.Run("defaults", func(t *testing.T) {
		t.Parallel()
		cfg, err := load(t, p2pCore)
		require.NoError(t, err)
		require.Equal(t, 100, cfg.MaxSubConns())
		require.Empty(t, cfg.CORSAllowedOrigins())
	})

	t.Run("overrides", func(t *testing.T) {
		t.Parallel()
		cfg, err := load(t, withOpts("/ip4/127.0.0.1/tcp/9000",
			"        maxSubConns: 7\n        corsAllowedOrigins: \" a, ,b \"\n"))
		require.NoError(t, err)
		require.Equal(t, 7, cfg.MaxSubConns())
		require.Equal(t, []string{"a", "b"}, cfg.CORSAllowedOrigins())
	})

	t.Run("empty cors", func(t *testing.T) {
		t.Parallel()
		cfg, err := load(t, withOpts("/ip4/127.0.0.1/tcp/9000", "        corsAllowedOrigins: \"\"\n"))
		require.NoError(t, err)
		require.Empty(t, cfg.CORSAllowedOrigins())
	})

	t.Run("invalid listen address", func(t *testing.T) {
		t.Parallel()
		_, err := load(t, withOpts("not-a-multiaddr", ""))
		require.ErrorContains(t, err, "failed parsing fsc.p2p.listenAddress [not-a-multiaddr]")
	})
}

func TestPublicKeyID(t *testing.T) {
	t.Parallel()
	raw := []byte("raw key")
	id, err := PKIDSynthesizer{}.PublicKeyID(raw)
	require.NoError(t, err)
	want := sha256.Sum256(raw)
	require.Equal(t, want[:], id)

	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	_, err = PKIDSynthesizer{}.PublicKeyID(pub)
	require.ErrorContains(t, err, "unsupported key type [ed25519.PublicKey]")
}
