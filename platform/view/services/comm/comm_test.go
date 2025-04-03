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

type node struct {
	commService *Service
	address     string
	pkID        view2.Identity
}

func TestWebsocketSession(t *testing.T) {
	RegisterTestingT(t)

	aliceConfig, bobConfig := getConfig("initiator"), getConfig("responder")

	router := &routing.StaticIDRouter{
		"alice": []host.PeerIPAddress{rest.ConvertAddress(aliceConfig.ListenAddress)},
		"bob":   []host.PeerIPAddress{rest.ConvertAddress(bobConfig.ListenAddress)},
	}
	alice := newWebsocketCommService(router, aliceConfig)
	alice.Start(context.Background())
	bob := newWebsocketCommService(router, bobConfig)
	bob.Start(context.Background())

	aliceNode := node{
		commService: alice,
		address:     aliceConfig.ListenAddress,
		pkID:        []byte("alice"),
	}
	bobNode := node{
		commService: bob,
		address:     bobConfig.ListenAddress,
		pkID:        []byte("bob"),
	}

	testExchange(aliceNode, bobNode)
}

func TestLibp2pSession(t *testing.T) {
	RegisterTestingT(t)

	aliceConfig, bobConfig := getConfig("initiator"), getConfig("responder")
	bobConfig.BootstrapNode = "alice"

	alice, alicePkID := newLibP2PCommService(aliceConfig, nil)
	alice.Start(context.Background())
	bob, bobPkID := newLibP2PCommService(bobConfig, &bootstrapNodeResolver{nodeID: alicePkID, nodeAddress: aliceConfig.ListenAddress})
	bob.Start(context.Background())

	aliceNode := node{
		commService: alice,
		address:     aliceConfig.ListenAddress,
		pkID:        alicePkID,
	}
	bobNode := node{
		commService: bob,
		address:     bobConfig.ListenAddress,
		pkID:        bobPkID,
	}

	testExchange(aliceNode, bobNode)
}

func testExchange(aliceNode, bobNode node) {
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
	}()

	wg.Wait()
}

func freeAddress() string {
	return fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", utils.MustGet(freeport.Take(1))[0])
}

func newWebsocketCommService(addresses *routing.StaticIDRouter, config *Config) *Service {
	discovery := routing.NewServiceDiscovery(addresses, routing.Random[host.PeerIPAddress]())

	pkiExtractor := endpoint.NewPKIExtractor()
	utils.Must(pkiExtractor.AddPublicKeyExtractor(&PKExtractor{}))
	pkiExtractor.SetPublicKeyIDSynthesizer(&rest.PKIDSynthesizer{})

	hostProvider := rest.NewEndpointBasedProvider(pkiExtractor, discovery, noop.NewTracerProvider(), websocket.NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}))

	return newService(hostProvider, nil, config, noop.NewTracerProvider(), &disabled.Provider{})
}

func newLibP2PCommService(config *Config, resolver EndpointService) (*Service, view2.Identity) {
	pkiExtractor := endpoint.NewPKIExtractor()
	utils.Must(pkiExtractor.AddPublicKeyExtractor(&PKExtractor{}))
	utils.Must(pkiExtractor.AddPublicKeyExtractor(endpoint2.PublicKeyExtractor{}))
	pkiExtractor.SetPublicKeyIDSynthesizer(&libp2p.PKIDSynthesizer{})

	hostProvider := libp2p.NewHostGeneratorProvider(&disabled.Provider{})

	pkID := pkiExtractor.ExtractPKI(utils.MustGet(x509.Serialize("", config.CertFile)))

	return newService(hostProvider, resolver, config, noop.NewTracerProvider(), &disabled.Provider{}), pkID
}

type bootstrapNodeResolver struct {
	nodeAddress string
	nodeID      []byte
}

func (r *bootstrapNodeResolver) Resolve(view2.Identity) (view2.Identity, map[view.PortName]string, []byte, error) {
	return nil, map[view.PortName]string{view.P2PPort: rest.ConvertAddress(r.nodeAddress)}, r.nodeID, nil
}

func (r *bootstrapNodeResolver) GetIdentity(string, []byte) (view2.Identity, error) {
	return view2.Identity("bstpnd"), nil
}
func getConfig(name string) *Config {
	rootPath, ok := os.LookupEnv("GOPATH")
	Expect(ok).To(BeFalse(), "GOPATH is not set")
	projectPath := path.Join(rootPath, "src", "github.com", "hyperledger-labs", "fabric-smart-client")
	mspDir := path.Join(projectPath, "integration", "fsc", "pingpong", "testdata", "fsc", "crypto", "peerOrganizations", "fsc.example.com", "peers", fmt.Sprintf("%s.fsc.example.com", name), "msp")
	return &Config{
		ListenAddress: freeAddress(),
		KeyFile:       path.Join(mspDir, "keystore", "priv_sk"),
		CertFile:      path.Join(mspDir, "signcerts", fmt.Sprintf("%s.fsc.example.com-cert.pem", name)),
	}
}
