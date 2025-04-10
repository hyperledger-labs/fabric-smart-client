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

type routerNodes struct {
	sender       *Service
	senderNode   Node
	receiverNode Node
	receiver     *Service
}

type benchmarkMetrics struct {
	latency time.Duration
}

const (
	numOfRoutes   int = 2
	numOfSessions int = 10
	numOfMsgs     int = 10
)

func createWebsocketRoutes() []routerNodes {
	routersNodes := make([]routerNodes, numOfRoutes)

	for routeNum := 0; routeNum < numOfRoutes; routeNum++ {
		senderConfig := GetConfig("initiator")
		receiverConfig := GetConfig("responder")

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
		routersNodes[routeNum] = routerNodes{sender: sender, senderNode: senderNode, receiver: receiver, receiverNode: receiverNode}
	}

	return routersNodes
}

func createLibp2pRoutes() []routerNodes {
	routersNodes := make([]routerNodes, numOfRoutes)
	for routeNum := 0; routeNum < numOfRoutes; routeNum++ {
		senderConfig := GetConfig("initiator")
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
		routersNodes[routeNum] = routerNodes{sender: sender, senderNode: senderNode, receiver: receiver, receiverNode: receiverNode}
	}

	return routersNodes
}

func testNodesExchange(routersNodes []routerNodes, protocol string) {
	metricsMap := map[string]benchmarkMetrics{}

	startTime := time.Now()
	for routeNum := 0; routeNum < numOfRoutes; routeNum++ {
		testExchangeMsgs(routersNodes[routeNum].senderNode, routersNodes[routeNum].receiverNode, metricsMap)
	}

	endTime := time.Now()

	for routeNum := 0; routeNum < numOfRoutes; routeNum++ {
		routersNodes[routeNum].sender.Stop()
		routersNodes[routeNum].receiver.Stop()
	}

	displayMetrics(endTime.Sub(startTime), metricsMap, protocol)
}

func BenchmarkTestWebsocket(b *testing.B) {
	RegisterTestingT(b)
	testNodesExchange(createWebsocketRoutes(), "Websocket")
}

func BenchmarkTestLibp2p(b *testing.B) {
	RegisterTestingT(b)
	testNodesExchange(createLibp2pRoutes(), "LipP2P")
}

func testExchangeMsgs(senderNode, receiverNode Node, metricsMap map[string]benchmarkMetrics) {
	wg := sync.WaitGroup{}
	wg.Add(2 * numOfSessions)

	sessions := make([]view.Session, 0, 2*numOfSessions+1)
	mu := sync.Mutex{}

	go senderExchangeMsgs(senderNode, receiverNode, &sessions, metricsMap, &mu, &wg)
	go receiverExchangeMsgs(receiverNode, &sessions, &mu, &wg)

	logger.Infof("Waiting on execution...")

	wg.Wait()
	logger.Infof("Execution finished. Closing sessions")
	for _, s := range sessions {
		s.Close()
	}
}

func senderExchangeMsgs(senderNode, receiverNode Node, sessions *[]view.Session, metricsMap map[string]benchmarkMetrics, mu *sync.Mutex, wg *sync.WaitGroup) {
	for sessId := 0; sessId < numOfSessions; sessId++ {
		defer wg.Done()
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

			metricsMap[senderSession.Info().ID+strconv.Itoa(sessId)+strconv.Itoa(msgId)] = benchmarkMetrics{latency: elapsed}
		}
		logger.Infof("Send EOF")
		Expect(senderSession.Send([]byte("EOF"))).To(Succeed())
		logger.Infof("Sent EOF")
	}
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
	var latency int64 = 0

	for _, metric := range metricsMap {
		latency = latency + int64(metric.latency)
	}

	averageLatency := latency / int64(len(metricsMap))
	averageThrouput := int64(duration) / int64(2*numOfMsgs)
	// Expect(senderSession.Info().EndpointPKID).To(Equal(receiverNode.pkID.Bytes()))

	fmt.Printf("============================= %s Metrics =============================\n", protocol)
	fmt.Printf("Number of routes: %d\n", numOfRoutes)
	fmt.Printf("Number of nodes: %d\n", numOfRoutes*2)
	fmt.Printf("Number of sessions per node: %d\n", numOfSessions)
	fmt.Printf("Number of Msgs per session: %d\n", 2*numOfMsgs)
	fmt.Printf("Average latency: %d (ms) \n", averageLatency)
	fmt.Printf("Average throuput: %d (msg/ms) \n", averageThrouput)
	fmt.Printf("Total run time: %d (ms) \n", duration)
}
