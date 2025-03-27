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
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
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

	addresses := map[string]string{
		"alice": freeAddress(),
		"bob":   freeAddress(),
	}
	routing := &routing.StaticIDRouter{
		"alice": []host.PeerIPAddress{rest.ConvertAddress(addresses["alice"])},
		"bob":   []host.PeerIPAddress{rest.ConvertAddress(addresses["bob"])},
	}
	rootPath, ok := os.LookupEnv("GOPATH")
	Expect(ok).To(BeFalse(), "GOPATH is not set")
	projectPath := path.Join(rootPath, "src", "github.com", "hyperledger-labs", "fabric-smart-client")
	testDataPath := path.Join(projectPath, "integration", "fsc", "pingpong", "testdata", "fsc", "crypto", "peerOrganizations", "fsc.example.com", "peers")
	aliceConfig := &Config{
		ListenAddress: addresses["alice"],
		KeyFile:       path.Join(testDataPath, "initiator.fsc.example.com", "msp", "keystore", "priv_sk"),
		CertFile:      path.Join(testDataPath, "initiator.fsc.example.com", "msp", "signcerts", "initiator.fsc.example.com-cert.pem"),
	}
	bobConfig := &Config{
		ListenAddress: addresses["bob"],
		KeyFile:       path.Join(testDataPath, "responder.fsc.example.com", "msp", "keystore", "priv_sk"),
		CertFile:      path.Join(testDataPath, "responder.fsc.example.com", "msp", "signcerts", "responder.fsc.example.com-cert.pem"),
	}
	alice := newWebsocketCommService(routing, aliceConfig)
	alice.Start(context.Background())
	bob := newWebsocketCommService(routing, bobConfig)
	bob.Start(context.Background())

	wg := sync.WaitGroup{}
	wg.Add(2)

	aliceSession, err := alice.NewSession("", "", rest.ConvertAddress(addresses["bob"]), []byte("bob"))
	Expect(err).ToNot(HaveOccurred())
	Expect(aliceSession.Info().Endpoint).To(Equal(rest.ConvertAddress(addresses["bob"])))
	Expect(aliceSession.Info().EndpointPKID).To(Equal([]byte("bob")))
	bobMasterSession, err := bob.MasterSession()
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

		bobSession, err := bob.NewSessionWithID(response.SessionID, "", response.FromEndpoint, response.FromPKID, nil, nil)
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

type noopEndpointService struct{}

func (s *noopEndpointService) Resolve(party view2.Identity) (view2.Identity, map[view.PortName]string, []byte, error) {
	return nil, nil, nil, nil
}
func (s *noopEndpointService) GetIdentity(label string, pkID []byte) (view2.Identity, error) {
	return nil, nil
}
