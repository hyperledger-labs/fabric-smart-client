/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashicorp/consul/sdk/freeport"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest/routing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest/websocket"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/mr-tron/base58/base58"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
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

func TestP2PLayerTestRound(t *testing.T) {
	bootstrapNode, node := setupTwoNodes(t)

	comm.P2PLayerTestRound(t, bootstrapNode, node)
}

func TestSessionsTestRound(t *testing.T) {
	bootstrapNode, node := setupTwoNodes(t)
	<-time.After(1 * time.Second)
	comm.SessionsTestRound(t, bootstrapNode, node)
}

func TestSessionsForMPCTestRound(t *testing.T) {
	bootstrapNode, node := setupTwoNodes(t)
	<-time.After(1 * time.Second)
	comm.SessionsForMPCTestRound(t, bootstrapNode, node)
}

func TestSessionsMultipleMessagesTestRound(t *testing.T) {
	bootstrapNode, node := setupTwoNodes(t)
	<-time.After(1 * time.Second)
	comm.SessionsMultipleMessagesTestRound(t, bootstrapNode, node)
}

func TestMTLSCallerIdentityBinding(t *testing.T) {
	bootstrapNode, node := setupTwoNodes(t)
	ctx := t.Context()
	bootstrapNode.Start(ctx)
	node.Start(ctx)
	defer bootstrapNode.Stop()
	defer node.Stop()

	session, err := bootstrapNode.NewSession("caller-view", "ctx", node.Address, []byte(node.ID))
	require.NoError(t, err)
	require.NotNil(t, session)

	require.NoError(t, session.Send([]byte("ping")))

	masterSession, err := node.MasterSession()
	require.NoError(t, err)

	msg := <-masterSession.Receive()
	require.NotNil(t, msg)
	require.Equal(t, []byte("ping"), msg.Payload)

	responder, err := node.NewSessionWithID(msg.SessionID, msg.ContextID, "", msg.FromPKID, view.Identity(msg.FromPKID), nil)
	require.NoError(t, err)
	require.True(t, responder.Info().Caller.Equal(view.Identity(msg.FromPKID)))

	maliciousCaller := view.Identity([]byte("malicious-caller"))
	_, err = node.NewSessionWithID(msg.SessionID, msg.ContextID, "", msg.FromPKID, maliciousCaller, nil)
	require.Error(t, err)
	require.True(t, responder.Info().Caller.Equal(view.Identity(msg.FromPKID)))

	require.NoError(t, responder.Send([]byte("pong")))
	reply := <-session.Receive()
	require.NotNil(t, reply)
	require.Equal(t, []byte("pong"), reply.Payload)
}

func setupTwoNodes(t *testing.T) (*comm.HostNode, *comm.HostNode) {
	tlsFiles := generateTLSFiles(t)

	bootstrapAddress := freeTCPAddress(t)
	otherAddress := freeTCPAddress(t)
	bootstrapCertPath := tlsFiles.bootstrapCert
	otherCertPath := tlsFiles.otherCert
	bootstrapID := mustPeerIDFromCert(t, bootstrapCertPath)
	otherID := mustPeerIDFromCert(t, otherCertPath)

	bootstrap, _ := newStaticRouteHostProvider(&routing.StaticIDRouter{
		bootstrapID: []host2.PeerIPAddress{bootstrapAddress},
		otherID:     []host2.PeerIPAddress{otherAddress},
	}, rest.NewConfigFromProperties(
		bootstrapAddress,
		tlsFiles.bootstrapKey,
		tlsFiles.bootstrapCert,
		[]string{tlsFiles.caCert},
		[]string{tlsFiles.caCert},
		true,
		100, nil,
	)).GetNewHost()
	bootstrapNode, err := comm.NewNode(t.Context(), bootstrap, &disabled.Provider{})
	require.NoError(t, err)

	other, _ := newStaticRouteHostProvider(&routing.StaticIDRouter{
		bootstrapID: []host2.PeerIPAddress{bootstrapAddress},
		otherID:     []host2.PeerIPAddress{otherAddress},
	}, rest.NewConfigFromProperties(
		otherAddress,
		tlsFiles.otherKey,
		tlsFiles.otherCert,
		[]string{tlsFiles.caCert},
		[]string{tlsFiles.caCert},
		true,
		100, nil,
	)).GetNewHost()
	otherNode, err := comm.NewNode(t.Context(), other, &disabled.Provider{})
	require.NoError(t, err)

	<-time.After(1 * time.Second)

	return &comm.HostNode{P2PNode: bootstrapNode, ID: bootstrapID, Address: bootstrapAddress},
		&comm.HostNode{P2PNode: otherNode, ID: otherID, Address: otherAddress}
}

func TestSessionsTwoNodesTestRound(t *testing.T) {
	bootstrapNode, node1, node2 := setupThreeNodes(t)
	comm.SessionsNodesTestRound(t, bootstrapNode, []*comm.HostNode{node1, node2}, 50)
}

func setupThreeNodes(t *testing.T) (*comm.HostNode, *comm.HostNode, *comm.HostNode) {
	// Create TLS certificates for three nodes: bootstrap, node1, node2
	dir := t.TempDir()

	// Create CA
	caPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "rest-test-ca"},
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
			ExtKeyUsage: []x509.ExtKeyUsage{
				x509.ExtKeyUsageServerAuth,
				x509.ExtKeyUsageClientAuth,
			},
			DNSNames:    []string{"localhost"},
			IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
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
	node1Cert, node1Key := makeNode(3, "node1", "node1-cert.pem", "node1-key.pem")
	node2Cert, node2Key := makeNode(4, "node2", "node2-cert.pem", "node2-key.pem")

	bootstrapID := mustPeerIDFromCert(t, bootstrapCert)
	node1ID := mustPeerIDFromCert(t, node1Cert)
	node2ID := mustPeerIDFromCert(t, node2Cert)

	bootstrapAddress := freeTCPAddress(t)
	node1Address := freeTCPAddress(t)
	node2Address := freeTCPAddress(t)

	bootstrapHost, _ := newStaticRouteHostProvider(&routing.StaticIDRouter{
		bootstrapID: []host2.PeerIPAddress{bootstrapAddress},
		node1ID:     []host2.PeerIPAddress{node1Address},
		node2ID:     []host2.PeerIPAddress{node2Address},
	}, rest.NewConfigFromProperties(
		bootstrapAddress,
		bootstrapKey,
		bootstrapCert,
		[]string{caCertPath},
		[]string{caCertPath},
		true,
		100, nil,
	)).GetNewHost()
	bootstrapNode, err := comm.NewNode(t.Context(), bootstrapHost, &disabled.Provider{})
	require.NoError(t, err)

	node1Host, _ := newStaticRouteHostProvider(&routing.StaticIDRouter{
		bootstrapID: []host2.PeerIPAddress{bootstrapAddress},
		node1ID:     []host2.PeerIPAddress{node1Address},
		node2ID:     []host2.PeerIPAddress{node2Address},
	}, rest.NewConfigFromProperties(
		node1Address,
		node1Key,
		node1Cert,
		[]string{caCertPath},
		[]string{caCertPath},
		true,
		100, nil,
	)).GetNewHost()
	node1Node, err := comm.NewNode(t.Context(), node1Host, &disabled.Provider{})
	require.NoError(t, err)

	node2Host, _ := newStaticRouteHostProvider(&routing.StaticIDRouter{
		bootstrapID: []host2.PeerIPAddress{bootstrapAddress},
		node1ID:     []host2.PeerIPAddress{node1Address},
		node2ID:     []host2.PeerIPAddress{node2Address},
	}, rest.NewConfigFromProperties(
		node2Address,
		node2Key,
		node2Cert,
		[]string{caCertPath},
		[]string{caCertPath},
		true,
		100, nil,
	)).GetNewHost()
	node2Node, err := comm.NewNode(t.Context(), node2Host, &disabled.Provider{})
	require.NoError(t, err)

	<-time.After(1 * time.Second)

	return &comm.HostNode{P2PNode: bootstrapNode, ID: bootstrapID, Address: bootstrapAddress},
		&comm.HostNode{P2PNode: node1Node, ID: node1ID, Address: node1Address},
		&comm.HostNode{P2PNode: node2Node, ID: node2ID, Address: node2Address}
}

func freeTCPAddress(t *testing.T) string {
	t.Helper()
	ports := freeport.GetT(t, 1)
	return fmt.Sprintf("127.0.0.1:%d", ports[0])
}

func mustPeerIDFromCert(t *testing.T, certPath string) string {
	t.Helper()
	raw, err := os.ReadFile(certPath)
	require.NoError(t, err)
	block, _ := pem.Decode(raw)
	require.NotNil(t, block)
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	marshaledPK, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	require.NoError(t, err)
	h := sha256.Sum256(marshaledPK)
	return base58.Encode(h[:])
}

type staticRoutHostProvider struct {
	routes *routing.StaticIDRouter
	config rest.Config
}

type generatedTLSFiles struct {
	caCert        string
	bootstrapCert string
	bootstrapKey  string
	otherCert     string
	otherKey      string
}

func generateTLSFiles(t *testing.T) generatedTLSFiles {
	t.Helper()
	dir := t.TempDir()

	caPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "rest-test-ca"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(2 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caPriv.PublicKey, caPriv)
	require.NoError(t, err)

	writePEM := func(path string, typ string, raw []byte) {
		f, err := os.Create(path)
		require.NoError(t, err)
		defer func() { require.NoError(t, f.Close()) }()
		require.NoError(t, pem.Encode(f, &pem.Block{Type: typ, Bytes: raw}))
	}

	caCertPath := filepath.Join(dir, "ca.pem")
	writePEM(caCertPath, "CERTIFICATE", caDER)

	makeNode := func(serial int64, cn, certName, keyName string) (string, string) {
		priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)

		certTemplate := &x509.Certificate{
			SerialNumber: big.NewInt(serial),
			Subject:      pkix.Name{CommonName: cn},
			NotBefore:    time.Now().Add(-time.Minute),
			NotAfter:     time.Now().Add(2 * time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
			ExtKeyUsage: []x509.ExtKeyUsage{
				x509.ExtKeyUsageServerAuth,
				x509.ExtKeyUsageClientAuth,
			},
			DNSNames:    []string{"localhost"},
			IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		}
		certDER, err := x509.CreateCertificate(rand.Reader, certTemplate, caTemplate, &priv.PublicKey, caPriv)
		require.NoError(t, err)

		certPath := filepath.Join(dir, certName)
		writePEM(certPath, "CERTIFICATE", certDER)

		keyRaw, err := x509.MarshalPKCS8PrivateKey(priv)
		require.NoError(t, err)
		keyPath := filepath.Join(dir, keyName)
		writePEM(keyPath, "PRIVATE KEY", keyRaw)
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

func TestSessionInfoSecurityGuarantees(t *testing.T) {
	ctx := t.Context()
	// Simpler: Alice, Bob and Charlie all share the same CA.
	allTlsFiles := generateThreeNodesTLSFiles(t)
	aliceNode, bobNode := setupTwoNodesFromTLS(t, allTlsFiles.alice, allTlsFiles.bob, allTlsFiles.caCert)
	aliceNode.Start(ctx)
	bobNode.Start(ctx)
	defer aliceNode.Stop()
	defer bobNode.Stop()

	// Alice initiates a session to Bob
	session, err := aliceNode.NewSession("alice-view", "ctx-1", bobNode.Address, []byte(bobNode.ID))
	require.NoError(t, err)
	require.NoError(t, session.Send([]byte("hello bob")))

	masterSession, err := bobNode.MasterSession()
	require.NoError(t, err)

	msg := <-masterSession.Receive()
	require.NotNil(t, msg)

	responder, err := bobNode.NewSessionWithID(msg.SessionID, msg.ContextID, "", msg.FromPKID, view.Identity(msg.FromPKID), nil)
	require.NoError(t, err)

	info := responder.Info()
	// Claim 1: Caller is the authenticated identity of the remote peer (Alice)
	require.Equal(t, view.Identity(aliceNode.ID), info.Caller, "Caller identity mismatch")
	// Claim 2: EndpointPKID is cryptographically verified and bound to transport identity
	require.Equal(t, []byte(aliceNode.ID), info.EndpointPKID, "EndpointPKID mismatch")

	charlieID := mustPeerIDFromCert(t, allTlsFiles.charlie.cert)
	charlieHost, _ := newStaticRouteHostProvider(&routing.StaticIDRouter{
		charlieID:  []host2.PeerIPAddress{freeTCPAddress(t)},
		bobNode.ID: []host2.PeerIPAddress{bobNode.Address},
	}, rest.NewConfigFromProperties(
		freeTCPAddress(t),
		allTlsFiles.charlie.key,
		allTlsFiles.charlie.cert,
		[]string{allTlsFiles.caCert},
		[]string{allTlsFiles.caCert},
		true,
		100, nil,
	)).GetNewHost()

	charlieP2PNode, err := comm.NewNode(t.Context(), charlieHost, &disabled.Provider{})
	require.NoError(t, err)
	charlieP2PNode.Start(ctx)
	defer charlieP2PNode.Stop()

	// Charlie tries to hijack
	charlieSession, err := charlieP2PNode.NewSessionWithID(msg.SessionID, msg.ContextID, bobNode.Address, []byte(bobNode.ID), nil, nil)
	require.NoError(t, err)
	// Charlie's Send might fail because Bob rejects the connection at the transport level (Identity Binding)
	_ = charlieSession.Send([]byte("i am alice"))

	// Bob should NOT receive Charlie's message in Alice's session
	select {
	case hijackedMsg := <-responder.Receive():
		t.Fatalf("Session hijacked! Received message from Charlie in Alice's session: %s", string(hijackedMsg.Payload))
	case <-time.After(1 * time.Second):
		// Success
	}
}

type nodeTLSFiles struct {
	cert string
	key  string
}

type threeNodesTLSFiles struct {
	caCert  string
	alice   nodeTLSFiles
	bob     nodeTLSFiles
	charlie nodeTLSFiles
}

func generateThreeNodesTLSFiles(t *testing.T) threeNodesTLSFiles {
	t.Helper()
	dir := t.TempDir()
	caPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "rest-test-ca"},
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

	makeNode := func(cn string) nodeTLSFiles {
		priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)

		certTemplate := &x509.Certificate{
			SerialNumber: big.NewInt(2),
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
		cPath := filepath.Join(dir, cn+".pem")
		writePEM(t, cPath, "CERTIFICATE", certDER)
		kRaw, _ := x509.MarshalPKCS8PrivateKey(priv)
		kPath := filepath.Join(dir, cn+".key")
		writePEM(t, kPath, "PRIVATE KEY", kRaw)
		return nodeTLSFiles{cert: cPath, key: kPath}
	}

	return threeNodesTLSFiles{
		caCert:  caCertPath,
		alice:   makeNode("alice"),
		bob:     makeNode("bob"),
		charlie: makeNode("charlie"),
	}
}

func setupTwoNodesFromTLS(t *testing.T, alice, bob nodeTLSFiles, caCert string) (*comm.HostNode, *comm.HostNode) {
	aliceAddr := freeTCPAddress(t)
	bobAddr := freeTCPAddress(t)
	aliceID := mustPeerIDFromCert(t, alice.cert)
	bobID := mustPeerIDFromCert(t, bob.cert)
	routes := &routing.StaticIDRouter{
		aliceID: []host2.PeerIPAddress{aliceAddr},
		bobID:   []host2.PeerIPAddress{bobAddr},
	}
	aliceH, _ := newStaticRouteHostProvider(routes, rest.NewConfigFromProperties(aliceAddr, alice.key, alice.cert, []string{caCert}, []string{caCert}, true, 100, nil)).GetNewHost()
	aliceNode, _ := comm.NewNode(t.Context(), aliceH, &disabled.Provider{})
	bobH, _ := newStaticRouteHostProvider(routes, rest.NewConfigFromProperties(bobAddr, bob.key, bob.cert, []string{caCert}, []string{caCert}, true, 100, nil)).GetNewHost()
	bobNode, _ := comm.NewNode(t.Context(), bobH, &disabled.Provider{})
	return &comm.HostNode{P2PNode: aliceNode, ID: aliceID, Address: aliceAddr},
		&comm.HostNode{P2PNode: bobNode, ID: bobID, Address: bobAddr}
}

func writePEM(t *testing.T, path string, typ string, raw []byte) {
	t.Helper()
	f, err := os.Create(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	require.NoError(t, pem.Encode(f, &pem.Block{Type: typ, Bytes: raw}))
}

func newStaticRouteHostProvider(routes *routing.StaticIDRouter, config rest.Config) *staticRoutHostProvider {
	return &staticRoutHostProvider{routes: routes, config: config}
}

func (p *staticRoutHostProvider) ExtraCAs() [][]byte {
	return nil
}

func (p *staticRoutHostProvider) GetNewHost() (host2.P2PHost, error) {
	nodeID, _ := p.routes.ReverseLookup(p.config.ListenAddress())
	discovery := routing.NewServiceDiscovery(p.routes, routing.RoundRobin[host2.PeerIPAddress]())
	return rest.NewHost(nodeID, discovery, websocket.NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0), p.config, p), nil
}
