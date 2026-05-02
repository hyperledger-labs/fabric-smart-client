/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	grpccomm "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/websocket/routing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func TestMain(m *testing.M) {
	spec := os.Getenv("FABRIC_LOGGING_SPEC")
	if len(spec) == 0 {
		spec = os.Getenv("FSC_LOGSPEC")
	}
	if len(spec) == 0 {
		spec = "error"
	}
	logging.Init(logging.Config{
		LogSpec: spec,
	})
	os.Exit(m.Run())
}

func TestP2PLayerTestRound(t *testing.T) { //nolint:paralleltest
	bootstrapNode, node := setupTwoNodes(t)
	comm.P2PLayerTestRound(t, bootstrapNode, node)
}

func TestSessionsTestRound(t *testing.T) { //nolint:paralleltest
	bootstrapNode, node := setupTwoNodes(t)
	comm.SessionsTestRound(t, bootstrapNode, node)
}

func TestSessionsForMPCTestRound(t *testing.T) { //nolint:paralleltest
	bootstrapNode, node := setupTwoNodes(t)
	comm.SessionsForMPCTestRound(t, bootstrapNode, node)
}

func TestSessionsMultipleMessagesTestRound(t *testing.T) { //nolint:paralleltest
	bootstrapNode, node := setupTwoNodes(t)
	comm.SessionsMultipleMessagesTestRound(t, bootstrapNode, node)
}

func TestMTLSCallerIdentityBinding(t *testing.T) { //nolint:paralleltest
	allTLS := generateThreeNodesTLSFiles(t)
	aliceNode, bobNode := setupTwoNodesFromTLS(t, allTLS.alice, allTLS.bob, allTLS.caCert)

	ctx := t.Context()
	aliceNode.Start(ctx)
	bobNode.Start(ctx)
	defer aliceNode.Stop()
	defer bobNode.Stop()

	session, err := aliceNode.NewSession("alice-view", "ctx-1", bobNode.Address, []byte(bobNode.ID))
	require.NoError(t, err)
	require.NoError(t, session.Send([]byte("hello bob")))

	masterSession, err := bobNode.MasterSession()
	require.NoError(t, err)

	msg := <-masterSession.Receive()
	require.NotNil(t, msg)

	responder, err := bobNode.NewResponderSession(msg.SessionID, msg.ContextID, "", msg.FromPKID, view.Identity(msg.FromPKID), nil)
	require.NoError(t, err)

	info := responder.Info()
	require.Equal(t, view.Identity(aliceNode.ID), info.Caller)
	require.Equal(t, []byte(aliceNode.ID), info.EndpointPKID)

	charlieAddr := freeTCPAddress(t)
	charlieClaim := aliceNode.ID
	routes := &routing.StaticIDRouter{
		bobNode.ID: []host2.PeerIPAddress{bobNode.Address},
	}
	charlieNode := newHostNode(t, charlieClaim, charlieAddr, allTLS.charlie, allTLS.caCert, routes)
	charlieNode.Start(ctx)
	defer charlieNode.Stop()

	charlieSession, err := charlieNode.NewResponderSession(msg.SessionID, msg.ContextID, bobNode.Address, []byte(bobNode.ID), nil, nil)
	require.NoError(t, err)
	_ = charlieSession.Send([]byte("i am alice"))

	select {
	case hijackedMsg := <-responder.Receive():
		t.Fatalf("session hijacked, received message from spoofed peer: %s", string(hijackedMsg.Payload))
	case <-time.After(1 * time.Second):
	}
}

type nodeTLSFiles struct {
	cert string
	key  string
}

type generatedTLSFiles struct {
	caCert        string
	bootstrapCert string
	bootstrapKey  string
	otherCert     string
	otherKey      string
}

type threeNodesTLSFiles struct {
	caCert  string
	alice   nodeTLSFiles
	bob     nodeTLSFiles
	charlie nodeTLSFiles
}

func setupTwoNodes(t *testing.T) (*comm.HostNode, *comm.HostNode) {
	t.Helper()
	tlsFiles := generateTLSFiles(t)
	bootstrapAddr := freeTCPAddress(t)
	otherAddr := freeTCPAddress(t)
	bootstrapID := mustPeerIDFromCert(t, tlsFiles.bootstrapCert)
	otherID := mustPeerIDFromCert(t, tlsFiles.otherCert)

	routes := &routing.StaticIDRouter{
		bootstrapID: []host2.PeerIPAddress{bootstrapAddr},
		otherID:     []host2.PeerIPAddress{otherAddr},
	}

	bootstrapNode := newHostNode(t, bootstrapID, bootstrapAddr, nodeTLSFiles{cert: tlsFiles.bootstrapCert, key: tlsFiles.bootstrapKey}, tlsFiles.caCert, routes)
	otherNode := newHostNode(t, otherID, otherAddr, nodeTLSFiles{cert: tlsFiles.otherCert, key: tlsFiles.otherKey}, tlsFiles.caCert, routes)

	time.Sleep(300 * time.Millisecond)

	return bootstrapNode, otherNode
}

func setupTwoNodesFromTLS(t *testing.T, alice, bob nodeTLSFiles, caCert string) (*comm.HostNode, *comm.HostNode) {
	t.Helper()
	aliceAddr := freeTCPAddress(t)
	bobAddr := freeTCPAddress(t)
	aliceID := mustPeerIDFromCert(t, alice.cert)
	bobID := mustPeerIDFromCert(t, bob.cert)
	routes := &routing.StaticIDRouter{
		aliceID: []host2.PeerIPAddress{aliceAddr},
		bobID:   []host2.PeerIPAddress{bobAddr},
	}
	return newHostNode(t, aliceID, aliceAddr, alice, caCert, routes),
		newHostNode(t, bobID, bobAddr, bob, caCert, routes)
}

func newHostNode(t *testing.T, nodeID, address string, tlsFiles nodeTLSFiles, caCert string, routes *routing.StaticIDRouter) *comm.HostNode {
	t.Helper()
	config := grpccomm.NewConfigFromProperties(address, tlsFiles.key, tlsFiles.cert, []string{caCert}, []string{caCert}, true, 5*time.Second)
	clientConfig, err := config.ClientConfig(nil)
	require.NoError(t, err)
	serverConfig, err := config.ServerConfig(nil)
	require.NoError(t, err)
	discovery := routing.NewServiceDiscovery(routes, routing.RoundRobin[host2.PeerIPAddress]())
	host, err := grpccomm.NewHost(nodeID, discovery, clientConfig, serverConfig, address)
	require.NoError(t, err)
	node, err := comm.NewNode(t.Context(), host, &disabled.Provider{})
	require.NoError(t, err)
	return &comm.HostNode{P2PNode: node, ID: nodeID, Address: address}
}

func freeTCPAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()
	return listener.Addr().String()
}

func mustPeerIDFromCert(t *testing.T, certPath string) string {
	t.Helper()
	raw, err := os.ReadFile(certPath)
	require.NoError(t, err)
	block, _ := pem.Decode(raw)
	require.NotNil(t, block)
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	id, err := grpccomm.PKIDSynthesizer{}.PublicKeyID(cert.PublicKey)
	require.NoError(t, err)
	return string(id)
}

func generateTLSFiles(t *testing.T) generatedTLSFiles {
	t.Helper()
	dir := t.TempDir()

	caPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "grpc-test-ca"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(2 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caPriv.PublicKey, caPriv)
	require.NoError(t, err)

	caCertPath := filepath.Join(dir, "ca.pem")
	writePEM(t, caCertPath, "CERTIFICATE", caDER)

	makeNode := func(serial int64, cn, certName, keyName string) (string, string) {
		priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)

		certTemplate := &x509.Certificate{
			SerialNumber: big.NewInt(serial),
			Subject:      pkix.Name{CommonName: cn},
			NotBefore:    time.Now().Add(-time.Minute),
			NotAfter:     time.Now().Add(2 * time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
			DNSNames:     []string{"localhost"},
			IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		}
		certDER, err := x509.CreateCertificate(rand.Reader, certTemplate, caTemplate, &priv.PublicKey, caPriv)
		require.NoError(t, err)

		certPath := filepath.Join(dir, certName)
		writePEM(t, certPath, "CERTIFICATE", certDER)

		keyRaw, err := x509.MarshalPKCS8PrivateKey(priv)
		require.NoError(t, err)
		keyPath := filepath.Join(dir, keyName)
		writePEM(t, keyPath, "PRIVATE KEY", keyRaw)
		return certPath, keyPath
	}

	bootstrapCert, bootstrapKey := makeNode(2, "bootstrap", "bootstrap-cert.pem", "bootstrap-key.pem")
	otherCert, otherKey := makeNode(3, "other", "other-cert.pem", "other-key.pem")

	return generatedTLSFiles{
		caCert:        caCertPath,
		bootstrapCert: bootstrapCert,
		bootstrapKey:  bootstrapKey,
		otherCert:     otherCert,
		otherKey:      otherKey,
	}
}

func generateThreeNodesTLSFiles(t *testing.T) threeNodesTLSFiles {
	t.Helper()
	dir := t.TempDir()
	caPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "grpc-test-ca"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(2 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caPriv.PublicKey, caPriv)
	require.NoError(t, err)
	caCertPath := filepath.Join(dir, "ca.pem")
	writePEM(t, caCertPath, "CERTIFICATE", caDER)

	makeNode := func(serial int64, cn string) nodeTLSFiles {
		priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)

		certTemplate := &x509.Certificate{
			SerialNumber: big.NewInt(serial),
			Subject:      pkix.Name{CommonName: cn},
			NotBefore:    time.Now().Add(-time.Minute),
			NotAfter:     time.Now().Add(2 * time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
			DNSNames:     []string{"localhost"},
			IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		}
		certDER, err := x509.CreateCertificate(rand.Reader, certTemplate, caTemplate, &priv.PublicKey, caPriv)
		require.NoError(t, err)
		certPath := filepath.Join(dir, cn+".pem")
		writePEM(t, certPath, "CERTIFICATE", certDER)
		keyRaw, err := x509.MarshalPKCS8PrivateKey(priv)
		require.NoError(t, err)
		keyPath := filepath.Join(dir, cn+".key")
		writePEM(t, keyPath, "PRIVATE KEY", keyRaw)
		return nodeTLSFiles{cert: certPath, key: keyPath}
	}

	return threeNodesTLSFiles{
		caCert:  caCertPath,
		alice:   makeNode(2, "alice"),
		bob:     makeNode(3, "bob"),
		charlie: makeNode(4, "charlie"),
	}
}

func writePEM(t *testing.T, path, typ string, raw []byte) {
	t.Helper()
	f, err := os.Create(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	require.NoError(t, pem.Encode(f, &pem.Block{Type: typ, Bytes: raw}))
}
