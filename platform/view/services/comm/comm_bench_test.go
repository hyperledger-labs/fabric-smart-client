/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest/routing"
	. "github.com/onsi/gomega"
)

// type (
const numOfConn int = 1

// 	const numOfMsg int = 1
// )

func BenchmarkTestWebsocketSession(b *testing.B) {
	RegisterTestingT(b)

	// senderNodes := []Node{}
	// receiverNodes := []Node{}

	for i := range numOfConn {

		senderConfig, receiverConfig := GetConfig("initiator"), GetConfig("responder")

		router := &routing.StaticIDRouter{
			"sender" + strconv.Itoa(i):   []host.PeerIPAddress{rest.ConvertAddress(senderConfig.ListenAddress)},
			"receiver" + strconv.Itoa(i): []host.PeerIPAddress{rest.ConvertAddress(receiverConfig.ListenAddress)},
		}

		// Need to move at the begining
		sender := NewWebsocketCommService(router, senderConfig)
		sender.Start(context.Background())
		receiver := NewWebsocketCommService(router, receiverConfig)
		receiver.Start(context.Background())

		senderNode := Node{
			commService: sender,
			address:     senderConfig.ListenAddress,
			pkID:        []byte("sender" + strconv.Itoa(i))}

		receiverNode := Node{
			commService: receiver,
			address:     receiverConfig.ListenAddress,
			pkID:        []byte("receiver" + strconv.Itoa(i))}

		benchmarkTestExchange(senderNode, receiverNode)
	}
}

func BenchmarkTestLibp2pSession(b *testing.B) {
	RegisterTestingT(b)

	for i := range numOfConn {
		senderConfig, receiverConfig := GetConfig("initiator"), GetConfig("responder")
		receiverConfig.BootstrapNode = "Sender" + strconv.Itoa(i)

		sender, senderPkID := NewLibP2PCommService(senderConfig, nil)
		sender.Start(context.Background())
		receiver, receiverPkID := NewLibP2PCommService(receiverConfig, &BootstrapNodeResolver{nodeID: senderPkID, nodeAddress: senderConfig.ListenAddress})
		receiver.Start(context.Background())

		senderNode := Node{
			commService: sender,
			address:     senderConfig.ListenAddress,
			pkID:        senderPkID,
		}
		receiverNode := Node{
			commService: receiver,
			address:     receiverConfig.ListenAddress,
			pkID:        receiverPkID,
		}

		benchmarkTestExchange(senderNode, receiverNode)
	}
}

func benchmarkTestExchange(senderNode, receiverNode Node) {
	wg := sync.WaitGroup{}
	wg.Add(2)

	//NewSessionWithID
	senderSession, err := senderNode.commService.NewSession("", "", rest.ConvertAddress(receiverNode.address), receiverNode.pkID)
	Expect(err).ToNot(HaveOccurred())
	Expect(senderSession.Info().Endpoint).To(Equal(rest.ConvertAddress(receiverNode.address)))
	Expect(senderSession.Info().EndpointPKID).To(Equal(receiverNode.pkID.Bytes()))
	recevierSession, err := receiverNode.commService.MasterSession()
	receiverNode, err := receiverNode.commService.NewSessionWithID(response.SessionID, "", response.FromEndpoint, response.FromPKID, nil, nil)
		
	Expect(err).ToNot(HaveOccurred())
	go func() {
		defer wg.Done()
		for i := 1; i <= 1; i++ {
			fmt.Print("---> Sender: sends request message\n")
			Expect(senderSession.Send([]byte("request"))).To(Succeed())

			fmt.Print("---> Sender: receives response message\n")
			response := <-senderSession.Receive()
			Expect(response).ToNot(BeNil())
			Expect(response).To(HaveField("Payload", Equal([]byte("response"))))
			senderSession.Close()
		}
	}()

	go func() {
		defer wg.Done()
		for i := 1; i <= 1; i++ {
			fmt.Print("---> Receiver: receive request message\n")
			response := <-recevierSession.Receive()
			Expect(response).ToNot(BeNil())
			Expect(response.Payload).To(Equal([]byte("request")))
			Expect(response.SessionID).To(Equal(senderSession.Info().ID))

			fmt.Print("---> Receiver: sends response message\n")
			Expect(err).ToNot(HaveOccurred())
			Expect(receiverNode.Send([]byte("response"))).To(Succeed())
		}
	}()

	wg.Wait()
}
