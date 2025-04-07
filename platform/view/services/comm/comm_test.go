/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"sync"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest/routing"
	. "github.com/onsi/gomega"
)

func TestWebsocketSession(t *testing.T) {
	RegisterTestingT(t)

	aliceConfig, bobConfig := GetConfig("initiator"), GetConfig("responder")

	router := &routing.StaticIDRouter{
		"alice": []host.PeerIPAddress{rest.ConvertAddress(aliceConfig.ListenAddress)},
		"bob":   []host.PeerIPAddress{rest.ConvertAddress(bobConfig.ListenAddress)},
	}
	alice := NewWebsocketCommService(router, aliceConfig)
	alice.Start(context.Background())
	bob := NewWebsocketCommService(router, bobConfig)
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

	testExchange(aliceNode, bobNode)
}

func TestLibp2pSession(t *testing.T) {
	RegisterTestingT(t)

	aliceConfig, bobConfig := GetConfig("initiator"), GetConfig("responder")
	bobConfig.BootstrapNode = "alice"

	alice, alicePkID := NewLibP2PCommService(aliceConfig, nil)
	alice.Start(context.Background())
	bob, bobPkID := NewLibP2PCommService(bobConfig, &BootstrapNodeResolver{nodeID: alicePkID, nodeAddress: aliceConfig.ListenAddress})
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
