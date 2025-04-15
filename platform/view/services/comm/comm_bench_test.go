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
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest/routing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	. "github.com/onsi/gomega"
)

const (
	numOfNodes    int = 2
	numOfSessions int = 1
	numOfMsgs     int = 1
)

type Nodes struct {
	sender       *Service
	senderNode   Node
	receiverNode Node
	receiver     *Service
}

type benchmarkMetrics struct {
	latency time.Duration
}

func createWebsocketNodes() []Nodes {

	var nodes []Nodes

	for senderNodeNum := 0; senderNodeNum < numOfNodes; senderNodeNum++ {

		senderConfig := GetConfig("initiator")

		var receiverConfigs []*Config
		var receivesIPAddress []host.PeerIPAddress
		for receivedNodeNum := 0; receivedNodeNum < numOfNodes; receivedNodeNum++ {
			if senderNodeNum == receivedNodeNum {
				continue
			}

			receiverConfig := GetConfig("responder")
			receivesIPAddress = append(receivesIPAddress, rest.ConvertAddress(receiverConfig.ListenAddress))
			receiverConfigs = append(receiverConfigs, receiverConfig)
		}

		router := &routing.StaticIDRouter{
			"sender":   []host.PeerIPAddress{rest.ConvertAddress(senderConfig.ListenAddress)},
			"receiver": receivesIPAddress}

		sender := NewWebsocketCommService(router, senderConfig.RestConfig())
		sender.Start(context.Background())

		senderNode := Node{
			commService: sender,
			address:     senderConfig.ListenAddress,
			pkID:        []byte("sender"),
		}

		for _, receiverConfig := range receiverConfigs {
			receiver := NewWebsocketCommService(router, receiverConfig.RestConfig())
			receiver.Start(context.Background())
			receiverNode := Node{
				commService: receiver,
				address:     receiverConfig.ListenAddress,
				pkID:        []byte("receiver"),
			}
			nodes = append(nodes, Nodes{sender: sender, senderNode: senderNode, receiver: receiver, receiverNode: receiverNode})
		}
	}
	return nodes
}

func createLibp2pNodes() []Nodes {

	var nodes []Nodes

	for senderNodeNum := 0; senderNodeNum < numOfNodes; senderNodeNum++ {

		senderConfig := GetConfig("initiator")

		for receivedNodeNum := 0; receivedNodeNum < numOfNodes; receivedNodeNum++ {
			if senderNodeNum == receivedNodeNum {
				continue
			}

			receiverConfig := GetConfig("responder")
			sender, senderPkID := NewLibP2PCommService(senderConfig.Libp2pConfig(""), senderConfig.CertFile, nil)
			sender.Start(context.Background())
			receiver, receiverPkID := NewLibP2PCommService(receiverConfig.Libp2pConfig("sender"), receiverConfig.CertFile, &BootstrapNodeResolver{nodeID: senderPkID, nodeAddress: senderConfig.ListenAddress})
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
			nodes = append(nodes, Nodes{sender: sender, senderNode: senderNode, receiver: receiver, receiverNode: receiverNode})
		}
	}

	return nodes
}

func testNodesExchange(nodes []Nodes, protocol string) {
	metricsMap := map[string]benchmarkMetrics{}

	mainwg := sync.WaitGroup{}
	startTime := time.Now()

	mainwg.Add(len(nodes))
	for _, exchangeNodes := range nodes {
		go testExchangeMsgs(exchangeNodes.senderNode, exchangeNodes.receiverNode, metricsMap, &mainwg)
	}
	mainwg.Wait()
	endTime := time.Now()

	for _, exchangeNodes := range nodes {
		exchangeNodes.sender.Stop()
		exchangeNodes.receiver.Stop()
	}

	displayMetrics(endTime.Sub(startTime), metricsMap, protocol)
}

func BenchmarkTestWebsocket(b *testing.B) {
	RegisterTestingT(b)
	testNodesExchange(createWebsocketNodes(), "Websocket")
}

func BenchmarkTestLibp2p(b *testing.B) {
	RegisterTestingT(b)
	testNodesExchange(createLibp2pNodes(), "LipP2P")
}

func testExchangeMsgs(senderNode, receiverNode Node, metricsMap map[string]benchmarkMetrics, mainwg *sync.WaitGroup) {
	defer mainwg.Done()

	wg := sync.WaitGroup{}
	wg.Add(2 * numOfSessions)

	//sessions := make([]view.Session, 0, 2*numOfSessions+1)
	var sessions []view.Session
	mu := sync.Mutex{}

	for sessId := 0; sessId < numOfSessions; sessId++ {
		go senderExchangeMsgs(senderNode, receiverNode, sessId, &sessions, metricsMap, &mu, &wg)
	}
	go receiverExchangeMsgs(receiverNode, &sessions, &mu, &wg)

	logger.Infof("Waiting on execution...")

	wg.Wait()

	logger.Infof("Execution finished. Closing sessions")
	for _, s := range sessions {
		s.Close()
	}
}

func senderExchangeMsgs(senderNode, receiverNode Node, sessId int, sessions *[]view.Session, metricsMap map[string]benchmarkMetrics, mu *sync.Mutex, wg *sync.WaitGroup) {
	logger.Infof("---> Create new session # %d", sessId)
	senderSession, err := senderNode.commService.NewSession("", "", rest.ConvertAddress(receiverNode.address), receiverNode.pkID)
	Expect(err).ToNot(HaveOccurred())
	mu.Lock()
	*sessions = append(*sessions, senderSession)
	mu.Unlock()
	Expect(senderSession.Info().Endpoint).To(Equal(rest.ConvertAddress(receiverNode.address)))
	Expect(senderSession.Info().EndpointPKID).To(Equal(receiverNode.pkID.Bytes()))
	logger.Infof("---> Created new session # %d [%v]", sessId, senderSession.Info())

	for msgId := 0; msgId < numOfMsgs; msgId++ {
		payload := fmt.Sprintf("request-%d-%d", sessId, msgId)
		logger.Infof("---> Sender: sends message [%s]", payload)
		start := time.Now()
		Expect(senderSession.Send([]byte(payload))).To(Succeed())
		logger.Infof("---> Sender: sent message [%s]", payload)
		response := <-senderSession.Receive()
		end := time.Now()
		elapsed := end.Sub(start)
		Expect(response).ToNot(BeNil())
		Expect(string(response.Payload)).To(Equal(fmt.Sprintf("response-%d-%d", sessId, msgId)))
		logger.Infof("---> Sender: received message: [%s]", string(response.Payload))

		_, ok := metricsMap[senderSession.Info().ID+strconv.Itoa(sessId)+strconv.Itoa(msgId)]
		if ok {
			Expect(ok).To(BeFalse(), "Metrics map keys should be unique")
		}

		metricsMap[senderSession.Info().ID+strconv.Itoa(sessId)+strconv.Itoa(msgId)] = benchmarkMetrics{latency: elapsed}
	}
	logger.Infof("Send EOF")
	Expect(senderSession.Send([]byte("EOF"))).To(Succeed())
	logger.Infof("Sent EOF")
	defer wg.Done()
}

func receiverExchangeMsgs(receiverNode Node, sessions *[]view.Session, mu *sync.Mutex, wg *sync.WaitGroup) {
	receiverMasterSession, err := receiverNode.commService.MasterSession()
	Expect(err).ToNot(HaveOccurred())

	mu.Lock()
	*sessions = append(*sessions, receiverMasterSession)
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
		session, err := receiverNode.commService.NewSessionWithID(response.SessionID, "", response.FromEndpoint, response.FromPKID, nil, nil)
		Expect(err).To(BeNil())
		logger.Infof("---> Receiver: opened session [%d] [%v]", sessId, session.Info())
		sessionMap[response.SessionID] = struct{}{}
		mu.Lock()
		*sessions = append(*sessions, session)
		mu.Unlock()

		go receiverSessionExchangeMsgs(sessId, session, wg)
	}
}

func receiverSessionExchangeMsgs(sessId int, session view.Session, wg *sync.WaitGroup) {
	defer wg.Done()
	payload := fmt.Sprintf("response-%d-0", sessId)
	logger.Infof("---> Receiver: Send message [%s]", payload)
	Expect(session.Send([]byte(payload))).To(Succeed())
	logger.Infof("---> Receiver: Sent message [%s]", payload)
	for response := range session.Receive() {
		if string(response.Payload) == "EOF" {
			logger.Infof("---> Receiver: Received EOF on [%d]", sessId)
			return
		}
		elements := strings.Split(string(response.Payload), "-")
		Expect(elements).To(HaveLen(3))
		sessId, msgId := utils.MustGet(strconv.Atoi(elements[1])), utils.MustGet(strconv.Atoi(elements[2]))
		payload := fmt.Sprintf("response-%d-%d", sessId, msgId)
		logger.Infof("---> Receiver: Send message [%s]", payload)
		Expect(session.Send([]byte(payload))).To(Succeed())
		logger.Infof("---> Receiver: Sent message [%s]", payload)
	}
}

func displayMetrics(duration time.Duration, metricsMap map[string]benchmarkMetrics, protocol string) {
	var latency time.Duration = 0

	for _, metric := range metricsMap {
		latency = latency + metric.latency
	}

	averageLatency := latency / time.Duration(len(metricsMap))
	throughput := float64(2*numOfMsgs) / float64(duration.Seconds())

	logger.Infof("============================= %s Metrics =============================\n", protocol)
	logger.Infof("Number of nodes: %d\n", numOfNodes*2)
	logger.Infof("Number of sessions per node: %d\n", numOfSessions)
	logger.Infof("Number of Msgs per session: %d\n", 2*numOfMsgs)
	logger.Infof("Average latency: %v\n", averageLatency)
	logger.Infof("Throughput: %f messages per second \n", throughput)
	logger.Infof("Total run time: %v\n", duration)
}
