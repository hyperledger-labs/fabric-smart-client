/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsc

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/require"

	nwocontext "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/context"
	nwonode "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
)

// tlsRenewFixture lays out, as cryptogen does, the TLS CA of org1.example.com, its PKCS#8 key,
// and alice's TLS pair signed by it, and returns the platform, alice and the CA pool.
func tlsRenewFixture(t *testing.T) (*Platform, *nwonode.Peer, *x509.CertPool) {
	t.Helper()
	newKey := func() *ecdsa.PrivateKey {
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		return k
	}
	pemOf := func(typ string, der []byte) []byte { return pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der}) }
	keyPEM := func(k *ecdsa.PrivateKey) []byte {
		der, err := x509.MarshalPKCS8PrivateKey(k)
		require.NoError(t, err)
		return pemOf("PRIVATE KEY", der)
	}

	caKey := newKey()
	caTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "tlsca.org1.example.com"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	caCert, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)

	nodeKey := newKey()
	nodeTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "alice.org1.example.com"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:    []string{"alice", "localhost"}, IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	nodeDER, err := x509.CreateCertificate(rand.Reader, nodeTmpl, caCert, &nodeKey.PublicKey, caKey)
	require.NoError(t, err)

	root := t.TempDir()
	p := NewPlatform(nwocontext.New(root, 0, nil), NewTopology(), nil)
	p.Organizations = []*nwonode.Organization{{Name: "Org1", Domain: "org1.example.com"}}
	peer := &nwonode.Peer{Name: "alice", Organization: "Org1"}
	caDir := filepath.Join(p.orgCryptoDir(peer), "tlsca")
	tlsDir := p.NodeLocalTLSDir(peer)
	for _, d := range []string{caDir, tlsDir} {
		require.NoError(t, os.MkdirAll(d, 0o755))
	}
	write := func(path string, data []byte) { require.NoError(t, os.WriteFile(path, data, 0o600)) }
	write(filepath.Join(caDir, "tlsca.org1.example.com-cert.pem"), pemOf("CERTIFICATE", caDER))
	write(filepath.Join(caDir, "priv_sk"), keyPEM(caKey))
	write(filepath.Join(tlsDir, "server.crt"), pemOf("CERTIFICATE", nodeDER))
	write(filepath.Join(tlsDir, "server.key"), keyPEM(nodeKey))

	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	return p, peer, pool
}

func TestRenewNodeTLSPair(t *testing.T) { //nolint:paralleltest // the platform's path helpers assert through the global Gomega handler
	gomega.RegisterTestingT(t)
	p, peer, pool := tlsRenewFixture(t)
	original, err := p.NodeTLSPair(peer)
	require.NoError(t, err)
	oldLeaf, err := parseCert(original.Cert)
	require.NoError(t, err)

	notAfter := time.Now().Add(48 * time.Hour).Truncate(time.Second)
	renewed, err := p.RenewNodeTLSPair(peer, notAfter)
	require.NoError(t, err)

	// The renewed pair loads, chains to the org's TLS CA, keeps the names the old one served,
	// and has the requested expiry and a new key.
	pair, err := tls.X509KeyPair(renewed.Cert, renewed.Key)
	require.NoError(t, err)
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	require.NoError(t, err)
	_, err = leaf.Verify(x509.VerifyOptions{Roots: pool, DNSName: "alice", KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
	require.NoError(t, err)
	require.Equal(t, oldLeaf.DNSNames, leaf.DNSNames)
	require.True(t, oldLeaf.IPAddresses[0].Equal(leaf.IPAddresses[0]))
	require.Equal(t, oldLeaf.Subject.CommonName, leaf.Subject.CommonName)
	require.True(t, notAfter.Equal(leaf.NotAfter))
	require.NotEqual(t, original.Key, renewed.Key)

	// Writing it replaces what the node serves; writing the original back restores it.
	require.NoError(t, p.WriteNodeTLSPair(peer, renewed))
	got, err := p.NodeTLSPair(peer)
	require.NoError(t, err)
	require.Equal(t, renewed, got)
	require.NoError(t, p.WriteNodeTLSPair(peer, original))
	got, err = p.NodeTLSPair(peer)
	require.NoError(t, err)
	require.Equal(t, original, got)
}
