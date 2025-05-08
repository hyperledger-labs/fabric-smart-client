/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest/routing"
	. "github.com/onsi/gomega"
)

const (
	// 	numOfNodes    int = 2
	numOfSessions int = 1
	numOfMsgs     int = 2
)

// func setUpCommServices() {

// }
func BenchmarkTestWebsocketSession(b *testing.B) {
	RegisterTestingT(b)

	senderConfig, receiverConfig := GetConfig("initiator"), GetConfig("responder")

	router := &routing.StaticIDRouter{
		"sender":   []host.PeerIPAddress{rest.ConvertAddress(senderConfig.ListenAddress)},
		"receiver": []host.PeerIPAddress{rest.ConvertAddress(receiverConfig.ListenAddress)},
	}
	sender := NewWebsocketCommService(router, senderConfig.RestConfig())
	sender.Start(context.Background())
	receiver := NewWebsocketCommService(router, receiverConfig.RestConfig())
	receiver.Start(context.Background())

	senderNode := Node{
		commService: sender,
		address:     senderConfig.ListenAddress,
		pkID:        []byte("sender"),
	}
	receiverNode := Node{
		commService: receiver,
		address:     senderConfig.ListenAddress,
		pkID:        []byte("receiver"),
	}
	benchmarkTestExchange(senderNode, receiverNode)
}

func benchmarkTestExchange(senderNode, receiverNode Node) {
	wg := sync.WaitGroup{}
	wg.Add(2 * numOfSessions)

	for j := 1; j <= numOfSessions; j++ {

		fmt.Printf("---> Create new session # %d \n", j)

		go func(sessionNum int) {
			defer wg.Done()

			senderSession, err := senderNode.commService.NewSession("", "", rest.ConvertAddress(receiverNode.address), receiverNode.pkID)
			Expect(err).ToNot(HaveOccurred())
			Expect(senderSession.Info().Endpoint).To(Equal(rest.ConvertAddress(receiverNode.address)))
			Expect(senderSession.Info().EndpointPKID).To(Equal(receiverNode.pkID.Bytes()))
			//defer senderSession.Close()

			for i := 1; i <= numOfMsgs; i++ {
				fmt.Printf("---> Sender: sends request message. Session #: %d, Msg #: %d \n", sessionNum, i)
				Expect(senderSession.Send([]byte("request"))).To(Succeed())
				//fmt.Printf("---> Sender: request message was sent successfully. Session #: %d, Msg #: %d \n", sessionNum, i)

				fmt.Printf("---> Sender: wait on receive response message. Session #: %d, Msg #: %d \n", sessionNum, i)
				response := <-senderSession.Receive()
				Expect(response).ToNot(BeNil())
				Expect(response).To(HaveField("Payload", Equal([]byte("response"))))
				//fmt.Printf("---> Sender: receives request message. Session #: %d, Msg #: %d \n", sessionNum, i)
			}
		}(j)

		go func(sessionNum int) {
			defer wg.Done()

			recevierMasterSession, err := receiverNode.commService.MasterSession()
			Expect(err).ToNot(HaveOccurred())
			//defer recevierMasterSession.Close()

			for i := 1; i <= numOfMsgs; i++ {
				fmt.Printf("---> Receiver: wait on receive request message. Session #: %d, Msg #: %d \n", sessionNum, i)
				response := <-recevierMasterSession.Receive()
				Expect(response).ToNot(BeNil())
				Expect(response.Payload).To(Equal([]byte("request")))
				//Expect(response.SessionID).To(Equal(senderSession.Info().ID))
				//fmt.Printf("---> Receiver: receives request message. Session #: %d, Msg #: %d \n", sessionNum, i)

				fmt.Printf("---> Receiver: sends response message. Session #: %d, Msg #: %d \n", sessionNum, i)
				receiverSession, err := receiverNode.commService.NewSessionWithID(response.SessionID, "", response.FromEndpoint, response.FromPKID, nil, nil)
				Expect(err).ToNot(HaveOccurred())
				Expect(receiverSession.Send([]byte("response"))).To(Succeed())
				//fmt.Printf("---> Receiver: response message was sent successfully. Session #: %d, Msg #: %d \n", sessionNum, i)
			}
		}(j)
	}
	wg.Wait()
}
