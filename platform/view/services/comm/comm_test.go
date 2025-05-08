/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"fmt"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/consul/sdk/freeport"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	endpoint2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/x509"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/libp2p"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest/routing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest/websocket"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	. "github.com/onsi/gomega"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestWebsocketSession(t *testing.T) {
	RegisterTestingT(t)

	aliceNode, bobNode := setupWebsocketSession()

	testExchange(aliceNode, bobNode)
}

// TestWebsocketSessionManySenders tests with multiple sender goroutines; creating a new session for every interaction
// TODO: current this test seems to deadlock, and cause the test to timeout;
// go test -v -count 10 -failfast -timeout 30s -run ^TestWebsocketSessionManySenders$
func TestWebsocketSessionManySenders(t *testing.T) {
	RegisterTestingT(t)

	numWorkers := 1
	numMessages := 1000

	aliceNode, bobNode := setupWebsocketSession()
	testExchangeManySenders(aliceNode, bobNode, numWorkers, numMessages)
	shutdown(aliceNode, bobNode)
}

func setupWebsocketSession() (Node, Node) {
	aliceConfig, bobConfig := GetConfig("initiator"), GetConfig("responder")

	router := &routing.StaticIDRouter{
		"alice": []host.PeerIPAddress{rest.ConvertAddress(aliceConfig.ListenAddress)},
		"bob":   []host.PeerIPAddress{rest.ConvertAddress(bobConfig.ListenAddress)},
	}
	alice := NewWebsocketCommService(router, aliceConfig.RestConfig())
	alice.Start(context.Background())
	bob := NewWebsocketCommService(router, bobConfig.RestConfig())
	bob.Start(context.Background())

	aliceNode := Node{
		commService: alice,
		address:     aliceConfig.ListenAddress,
		pkID:        []byte("alice"),
	}
	bobNode := Node{
		commService: bob,
		address:     bobConfig.ListenAddress,
		pkID:        []byte("bob"),
	}

	return aliceNode, bobNode
}

func shutdown(nodes ...Node) {
	// TODO: how to check that the comm service is actually stopped?
	for _, n := range nodes {
		n.commService.Stop()
	}
	// until we figure out how to check when the comm service has stopped completely we give it a bit of time to ZzzZzz
	time.Sleep(1 * time.Second)
}

func TestLibp2pSession(t *testing.T) {
	RegisterTestingT(t)

	aliceConfig, bobConfig := GetConfig("initiator"), GetConfig("responder")

	alice, alicePkID := NewLibP2PCommService(aliceConfig.Libp2pConfig(""), aliceConfig.CertFile, nil)
	alice.Start(context.Background())
	bob, bobPkID := NewLibP2PCommService(bobConfig.Libp2pConfig("alice"), bobConfig.CertFile, &BootstrapNodeResolver{nodeID: alicePkID, nodeAddress: aliceConfig.ListenAddress})
	bob.Start(context.Background())

	aliceNode := Node{
		commService: alice,
		address:     aliceConfig.ListenAddress,
		pkID:        alicePkID,
	}
	bobNode := Node{
		commService: bob,
		address:     bobConfig.ListenAddress,
		pkID:        bobPkID,
	}

	testExchange(aliceNode, bobNode)
}

func testExchange(aliceNode, bobNode Node) {
	wg := sync.WaitGroup{}
	wg.Add(2)

	aliceSession, err := aliceNode.commService.NewSession("", "", rest.ConvertAddress(bobNode.address), bobNode.pkID)
	Expect(err).ToNot(HaveOccurred())
	Expect(aliceSession.Info().Endpoint).To(Equal(rest.ConvertAddress(bobNode.address)))
	Expect(aliceSession.Info().EndpointPKID).To(Equal(bobNode.pkID.Bytes()))
	bobMasterSession, err := bobNode.commService.MasterSession()
	Expect(err).ToNot(HaveOccurred())
	go func() {
		defer wg.Done()
		Expect(aliceSession.Send([]byte("msg1"))).To(Succeed())
		Expect(aliceSession.Send([]byte("msg2"))).To(Succeed())
		response := <-aliceSession.Receive()
		Expect(response).ToNot(BeNil())
		Expect(response).To(HaveField("Payload", Equal([]byte("msg3"))))
		Expect(aliceSession.Send([]byte("msg4"))).To(Succeed())
	}()

	go func() {
		defer wg.Done()
		response := <-bobMasterSession.Receive()
		Expect(response).ToNot(BeNil())
		Expect(response.Payload).To(Equal([]byte("msg1")))
		Expect(response.SessionID).To(Equal(aliceSession.Info().ID))

		response = <-bobMasterSession.Receive()
		Expect(response).ToNot(BeNil())
		Expect(response.Payload).To(Equal([]byte("msg2")))

		bobSession, err := bobNode.commService.NewSessionWithID(response.SessionID, "", response.FromEndpoint, response.FromPKID, nil, nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(bobSession.Send([]byte("msg3"))).To(Succeed())
		response = <-bobSession.Receive()
		Expect(response).ToNot(BeNil())
		Expect(response.Payload).To(Equal([]byte("msg4")))
	}()

	wg.Wait()
}

func testExchangeManySenders(aliceNode, bobNode Node, numWorker, numOfMsgs int) {
	var wgBob sync.WaitGroup

	ctx, cancel := context.WithCancel(context.Background())

	// setup bob as our receiver
	bobMasterSession, err := bobNode.commService.MasterSession()
	Expect(err).ToNot(HaveOccurred())
	wgBob.Add(1)
	go func() {
		defer wgBob.Done()
		for {
			select {
			// run until we close via the context
			case <-ctx.Done():
				return
			case response := <-bobMasterSession.Receive():
				// get our message from master session
				Expect(response).ToNot(BeNil())
				Expect(response.Payload).To(Equal([]byte("ping")))

				// create a response session
				bobSession, err := bobNode.commService.NewSessionWithID(response.SessionID, "", response.FromEndpoint, response.FromPKID, nil, nil)
				Expect(err).ToNot(HaveOccurred())
				Expect(bobSession.Send([]byte("pong"))).To(Succeed())

				// close it
				bobSession.Close()
				Eventually(bobSession.Info().Closed).Should(BeTrue())
			}
		}
	}()

	// setup alice our sender
	var wgAlice sync.WaitGroup
	for i := 0; i <= numWorker; i++ {
		wgAlice.Add(1)
		go func() {
			defer wgAlice.Done()

			// we send every message in a fresh session
			for j := 0; j <= numOfMsgs; j++ {
				// setup
				aliceSession, err := aliceNode.commService.NewSession("", "", rest.ConvertAddress(bobNode.address), bobNode.pkID)
				Expect(err).ToNot(HaveOccurred())
				Expect(aliceSession.Info().Endpoint).To(Equal(rest.ConvertAddress(bobNode.address)))
				Expect(aliceSession.Info().EndpointPKID).To(Equal(bobNode.pkID.Bytes()))

				// send
				Eventually(aliceSession.Send([]byte("ping"))).Should(Succeed())

				// receive
				Eventually(func(g Gomega) {
					response := <-aliceSession.Receive()
					g.Expect(response).ToNot(BeNil())
					g.Expect(response).To(HaveField("Payload", Equal([]byte("pong"))))
				}).Should(Succeed())

				// close
				aliceSession.Close()
				Eventually(aliceSession.Info().Closed).Should(BeTrue())
			}
		}()
	}

	wgAlice.Wait()
	cancel()
	wgBob.Wait()

	bobMasterSession.Close()
	Eventually(bobMasterSession.Info().Closed).Should(BeTrue())
}

func freeAddress() string {
	return fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", utils.MustGet(freeport.Take(1))[0])
}

func NewWebsocketCommService(addresses *routing.StaticIDRouter, config rest.Config) *Service {
	discovery := routing.NewServiceDiscovery(addresses, routing.Random[host.PeerIPAddress]())

	pkiExtractor := endpoint.NewPKIExtractor()
	utils.Must(pkiExtractor.AddPublicKeyExtractor(&PKExtractor{}))
	pkiExtractor.SetPublicKeyIDSynthesizer(&rest.PKIDSynthesizer{})

	hostProvider := rest.NewEndpointBasedProvider(config, pkiExtractor, discovery, noop.NewTracerProvider(), websocket.NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}))

	return utils.MustGet(NewService(hostProvider, noop.NewTracerProvider(), &disabled.Provider{}))
}

func NewLibP2PCommService(config libp2p.Config, certPath string, resolver EndpointService) (*Service, view2.Identity) {
	pkiExtractor := endpoint.NewPKIExtractor()
	utils.Must(pkiExtractor.AddPublicKeyExtractor(&PKExtractor{}))
	utils.Must(pkiExtractor.AddPublicKeyExtractor(endpoint2.PublicKeyExtractor{}))
	pkiExtractor.SetPublicKeyIDSynthesizer(&libp2p.PKIDSynthesizer{})

	hostProvider := libp2p.NewHostGeneratorProvider(config, &disabled.Provider{}, resolver)

	pkID := pkiExtractor.ExtractPKI(utils.MustGet(x509.Serialize("", certPath)))

	return utils.MustGet(NewService(hostProvider, noop.NewTracerProvider(), &disabled.Provider{})), pkID
}

type BootstrapNodeResolver struct {
	nodeAddress string
	nodeID      []byte
}

func (r *BootstrapNodeResolver) Resolve(view2.Identity) (view2.Identity, map[view.PortName]string, []byte, error) {
	return nil, map[view.PortName]string{view.P2PPort: rest.ConvertAddress(r.nodeAddress)}, r.nodeID, nil
}

func (r *BootstrapNodeResolver) GetIdentity(string, []byte) (view2.Identity, error) {
	return view2.Identity("bstpnd"), nil
}

type Config struct {
	ListenAddress  host.PeerIPAddress
	PrivateKeyFile string
	CertFile       string
}

func (c *Config) RestConfig() rest.Config {
	return rest.NewConfigFromProperties(c.ListenAddress, c.PrivateKeyFile, c.CertFile)
}

func (c *Config) Libp2pConfig(bootstrap host.PeerIPAddress) libp2p.Config {
	return libp2p.NewConfigFromProperties(c.ListenAddress, bootstrap, c.PrivateKeyFile)
}

func GetConfig(name string) *Config {
	rootPath, ok := os.LookupEnv("GOPATH")
	if !ok {
		panic("gopath not set")
	}
	projectPath := path.Join(rootPath, "src", "github.com", "hyperledger-labs", "fabric-smart-client")
	mspDir := path.Join(projectPath, "integration", "fsc", "pingpong", "testdata", "fsc", "crypto", "peerOrganizations", "fsc.example.com", "peers", fmt.Sprintf("%s.fsc.example.com", name), "msp")
	return &Config{
		ListenAddress:  freeAddress(),
		PrivateKeyFile: path.Join(mspDir, "keystore", "priv_sk"),
		CertFile:       path.Join(mspDir, "signcerts", fmt.Sprintf("%s.fsc.example.com-cert.pem", name)),
	}
}
