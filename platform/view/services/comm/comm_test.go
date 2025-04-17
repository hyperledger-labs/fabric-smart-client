/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest/routing"
	. "github.com/onsi/gomega"
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
