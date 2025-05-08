/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest/routing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	. "github.com/onsi/gomega"
)

const (
	// 	numOfNodes    int = 2
	numOfSessions int = 3
	numOfMsgs     int = 20
)

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
		address:     receiverConfig.ListenAddress,
		pkID:        []byte("receiver"),
	}

	benchmarkTestExchange(senderNode, receiverNode)
	sender.Stop()
	receiver.Stop()
}

func benchmarkTestExchange(senderNode, receiverNode Node) {
	wg := sync.WaitGroup{}
	wg.Add(2 * numOfSessions)

	sessions := make([]view.Session, 0, 2*numOfSessions+1)
	mu := sync.Mutex{}

	for sessId := 0; sessId < numOfSessions; sessId++ {

		go func(sessId int) {
			logger.Infof("---> Create new session # %d", sessId)
			senderSession, err := senderNode.commService.NewSession("", "", rest.ConvertAddress(receiverNode.address), receiverNode.pkID)
			Expect(err).ToNot(HaveOccurred())
			mu.Lock()
			sessions = append(sessions, senderSession)
			mu.Unlock()
			Expect(senderSession.Info().Endpoint).To(Equal(rest.ConvertAddress(receiverNode.address)))
			Expect(senderSession.Info().EndpointPKID).To(Equal(receiverNode.pkID.Bytes()))
			logger.Infof("---> Created new session # %d [%v]", sessId, senderSession.Info())

			for msgId := 0; msgId < numOfMsgs; msgId++ {
				payload := fmt.Sprintf("request-%d-%d", sessId, msgId)
				logger.Infof("---> Sender: sends message [%s]", payload)
				Expect(senderSession.Send([]byte(payload))).To(Succeed())
				logger.Infof("---> Sender: sent message [%s]", payload)

				response := <-senderSession.Receive()
				Expect(response).ToNot(BeNil())
				Expect(string(response.Payload)).To(Equal(fmt.Sprintf("response-%d-%d", sessId, msgId)))
				logger.Infof("---> Sender: received message: [%s]", string(response.Payload))
			}
			logger.Infof("Send EOF")
			Expect(senderSession.Send([]byte("EOF"))).To(Succeed())
			logger.Infof("Sent EOF")
			wg.Done()
		}(sessId)
	}

	go func() {
		receiverMasterSession, err := receiverNode.commService.MasterSession()
		Expect(err).ToNot(HaveOccurred())

		mu.Lock()
		sessions = append(sessions, receiverMasterSession)
		mu.Unlock()
		sessionMap := map[string]struct{}{}

		logger.Infof("---> Receiver: start receiving on master session")
		for response := range receiverMasterSession.Receive() {
			Expect(response).ToNot(BeNil())
			logger.Infof("---> Receiver: received message [%s]", string(response.Payload))
			Expect(string(response.Payload)).To(ContainSubstring("request"))
			elements := strings.Split(string(response.Payload), "-")
			Expect(elements).To(HaveLen(3))
			sessId, msgId := utils.MustGet(strconv.Atoi(elements[1])), utils.MustGet(strconv.Atoi(elements[2]))
			Expect(msgId).To(Equal(0))

			_, ok := sessionMap[response.SessionID]
			Expect(ok).To(BeFalse(), "we should not receive a second message on the master session [%s]", string(response.Payload))
			logger.Infof("---> Receiver: open session [%d]", sessId)
			sess, err := receiverNode.commService.NewSessionWithID(response.SessionID, "", response.FromEndpoint, response.FromPKID, nil, nil)
			Expect(err).To(BeNil())
			logger.Infof("---> Receiver: opened session [%d] [%v]", sessId, sess.Info())
			sessionMap[response.SessionID] = struct{}{}
			mu.Lock()
			sessions = append(sessions, sess)
			mu.Unlock()
			go func(sess view.Session, sessId int) {
				defer wg.Done()
				payload := fmt.Sprintf("response-%d-0", sessId)
				logger.Infof("---> Receiver: Send message [%s]", payload)
				Expect(sess.Send([]byte(payload))).To(Succeed())
				logger.Infof("---> Receiver: Sent message [%s]", payload)
				for response := range sess.Receive() {
					if string(response.Payload) == "EOF" {
						logger.Infof("---> Receiver: Received EOF on [%d]", sessId)
						return
					}
					elements := strings.Split(string(response.Payload), "-")
					Expect(elements).To(HaveLen(3))
					sessId, msgId := utils.MustGet(strconv.Atoi(elements[1])), utils.MustGet(strconv.Atoi(elements[2]))
					payload := fmt.Sprintf("response-%d-%d", sessId, msgId)
					logger.Infof("---> Receiver: Send message [%s]", payload)
					Expect(sess.Send([]byte(payload))).To(Succeed())
					logger.Infof("---> Receiver: Sent message [%s]", payload)
				}
			}(sess, sessId)
		}
	}()
	logger.Infof("Waiting on execution...")

	wg.Wait()
	logger.Infof("Execution finished. Closing sessions")
	for _, s := range sessions {
		s.Close()
	}
}
