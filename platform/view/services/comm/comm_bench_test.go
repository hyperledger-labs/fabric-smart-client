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
	numOfNodes    int = 5
	numOfSessions int = 1
	numOfMsgs     int = 1
)

type connection struct {
	senderNode   *Node
	receiverNode *Node
}

type benchmarkMetrics struct {
	latency time.Duration
}

func createWebsocketConnections() []*connection {

	router := routing.StaticIDRouter{}
	var configs []*Config
	var nodeIds []string

	for nodeNum := 0; nodeNum < numOfNodes; nodeNum++ {
		nodeId := fmt.Sprintf("Node_%d", nodeNum)
		nodeIds = append(nodeIds, nodeId)
		config := GetConfig("initiator")

		logger.Infof("Creating node=%s, address=%s", nodeId, config.ListenAddress)

		configs = append(configs, config)
		router[nodeId] = []host.PeerIPAddress{rest.ConvertAddress(config.ListenAddress)}
	}

	var nodes []*Node
	for nodeNum := 0; nodeNum < numOfNodes; nodeNum++ {
		service := NewWebsocketCommService(&router, configs[nodeNum].RestConfig())
		service.Start(context.Background())
		node := Node{
			commService: service,
			address:     configs[nodeNum].ListenAddress,
			pkID:        []byte(nodeIds[nodeNum]),
		}
		nodes = append(nodes, &node)
	}

	// Mesh
	var connections []*connection
	for senderNodeNum := 0; senderNodeNum < numOfNodes; senderNodeNum++ {
		sender := nodes[senderNodeNum]
		for receiverNodeNum := 0; receiverNodeNum < numOfNodes; receiverNodeNum++ {
			if senderNodeNum == receiverNodeNum {
				continue
			}
			receiver := nodes[receiverNodeNum]
			connections = append(connections, &connection{senderNode: sender, receiverNode: receiver})
		}
	}

	// Mesh
	// A-->B, C
	// B-->A, C
	// C-->B, A

	// Star
	// A, B, C
	// A ->B, C
	// B ->A
	// C ->A

	return connections
}

// func createLibp2pNodes() []Nodes {

// 	var nodes []Nodes

// 	for senderNodeNum := 0; senderNodeNum < numOfNodes; senderNodeNum++ {

// 		senderConfig := GetConfig("initiator")

// 		for receivedNodeNum := 0; receivedNodeNum < numOfNodes; receivedNodeNum++ {
// 			if senderNodeNum == receivedNodeNum {
// 				continue
// 			}

// 			receiverConfig := GetConfig("responder")
// 			receiverConfig.BootstrapNode = "sender"
// 			sender, senderPkID := NewLibP2PCommService(senderConfig, nil)
// 			sender.Start(context.Background())
// 			receiver, receiverPkID := NewLibP2PCommService(receiverConfig, &BootstrapNodeResolver{nodeID: senderPkID, nodeAddress: senderConfig.ListenAddress})
// 			receiver.Start(context.Background())

// 			senderNode := Node{
// 				commService: sender,
// 				address:     senderConfig.ListenAddress,
// 				pkID:        senderPkID,
// 			}

// 			receiverNode := Node{
// 				commService: receiver,
// 				address:     receiverConfig.ListenAddress,
// 				pkID:        receiverPkID,
// 			}
// 			nodes = append(nodes, Nodes{sender: sender, senderNode: senderNode, receiver: receiver, receiverNode: receiverNode})
// 		}
// 	}

// 	return nodes
// }

func testNodesExchange(connections []*connection, protocol string) {
	metricsMap := map[string]benchmarkMetrics{}

	mainwg := sync.WaitGroup{}
	startTime := time.Now()

	connNum := 0
	for _, connection := range connections {
		mainwg.Add(1)
		go testExchangeMsgs(connNum, *connection.senderNode, *connection.receiverNode, metricsMap, &mainwg)
		connNum++
	}
	mainwg.Wait()
	endTime := time.Now()

	for connNum := 0; connNum < len(connections); connNum = (connNum + numOfNodes - 1) {
		connections[connNum].senderNode.commService.Stop()
	}

	displayMetrics(endTime.Sub(startTime), metricsMap, protocol)
}

func BenchmarkTestWebsocket(b *testing.B) {
	RegisterTestingT(b)
	testNodesExchange(createWebsocketConnections(), "Websocket")
}

// func BenchmarkTestLibp2p(b *testing.B) {
// 	RegisterTestingT(b)
// 	testNodesExchange(createLibp2pNodes(), "LipP2P")
// }

func testExchangeMsgs(connNum int, senderNode, receiverNode Node, metricsMap map[string]benchmarkMetrics, mainwg *sync.WaitGroup) {
	defer mainwg.Done()

	wg := sync.WaitGroup{}

	var sessions []view.Session
	mu := sync.Mutex{}

	for sessId := 0; sessId < numOfSessions; sessId++ {
		wg.Add(1)
		go senderExchangeMsgs(connNum, senderNode, receiverNode, sessId, &sessions, metricsMap, &mu, &wg)
	}
	go receiverExchangeMsgs(connNum, receiverNode, &sessions, &mu, &wg)

	logger.Infof("Waiting on execution...")

	wg.Wait()

	logger.Infof("Execution finished. Closing sessions")
	for _, s := range sessions {
		s.Close()
	}
}

func senderExchangeMsgs(connNum int, senderNode, receiverNode Node, sessId int, sessions *[]view.Session, metricsMap map[string]benchmarkMetrics, mu *sync.Mutex, wg *sync.WaitGroup) {
	defer wg.Done()
	logger.Infof("ConnNum = %d ---> Create new session # %d", connNum, sessId)
	senderSession, err := senderNode.commService.NewSession("", "", rest.ConvertAddress(receiverNode.address), receiverNode.pkID)
	Expect(err).ToNot(HaveOccurred())
	mu.Lock()
	*sessions = append(*sessions, senderSession)
	mu.Unlock()
	Expect(senderSession.Info().Endpoint).To(Equal(rest.ConvertAddress(receiverNode.address)))
	Expect(senderSession.Info().EndpointPKID).To(Equal(receiverNode.pkID.Bytes()))
	logger.Infof("ConnNum = %d ---> Created new session # %d [%v]", connNum, sessId, senderSession.Info())

	for msgId := 0; msgId < numOfMsgs; msgId++ {
		payload := fmt.Sprintf("request-%d-%d", sessId, msgId)
		logger.Infof("ConnNum = %d ---> Sender: sends message [%s]", connNum, payload)
		start := time.Now()
		Expect(senderSession.Send([]byte(payload))).To(Succeed())
		logger.Infof("ConnNum = %d ---> Sender: sent message [%s]", connNum, payload)
		response := <-senderSession.Receive()
		end := time.Now()
		elapsed := end.Sub(start)
		Expect(response).ToNot(BeNil())
		Expect(string(response.Payload)).To(Equal(fmt.Sprintf("response-%d-%d", sessId, msgId)))
		logger.Infof("ConnNum = %d ---> Sender: received message: [%s]", connNum, string(response.Payload))

		_, ok := metricsMap[strconv.Itoa(connNum)+senderSession.Info().ID+strconv.Itoa(sessId)+strconv.Itoa(msgId)]
		if ok {
			Expect(ok).To(BeFalse(), "Metrics map keys should be unique")
		}
		metricsMap[strconv.Itoa(connNum)+senderSession.Info().ID+strconv.Itoa(sessId)+strconv.Itoa(msgId)] = benchmarkMetrics{latency: elapsed}
	}
	logger.Infof("Send EOF")
	Expect(senderSession.Send([]byte("EOF"))).To(Succeed())
	logger.Infof("Sent EOF")
}

func receiverExchangeMsgs(connNum int, receiverNode Node, sessions *[]view.Session, mu *sync.Mutex, wg *sync.WaitGroup) {
	receiverMasterSession, err := receiverNode.commService.MasterSession()
	Expect(err).ToNot(HaveOccurred())

	mu.Lock()
	*sessions = append(*sessions, receiverMasterSession)
	mu.Unlock()
	sessionMap := map[string]struct{}{}

	logger.Infof("ConnNum = %d ---> Receiver: start receiving on master session", connNum)
	for response := range receiverMasterSession.Receive() {
		Expect(response).ToNot(BeNil())
		logger.Infof("ConnNum = %d ---> Receiver: received message [%s]", connNum, string(response.Payload))
		Expect(string(response.Payload)).To(ContainSubstring("request"))
		elements := strings.Split(string(response.Payload), "-")
		Expect(elements).To(HaveLen(3))
		sessId, msgId := utils.MustGet(strconv.Atoi(elements[1])), utils.MustGet(strconv.Atoi(elements[2]))
		Expect(msgId).To(Equal(0))

		_, ok := sessionMap[response.SessionID]
		Expect(ok).To(BeFalse(), "we should not receive a second message on the master session [%s]", string(response.Payload))
		logger.Infof("ConnNum = %d ---> Receiver: open session [%d]", connNum, sessId)
		session, err := receiverNode.commService.NewSessionWithID(response.SessionID, "", response.FromEndpoint, response.FromPKID, nil, nil)
		Expect(err).To(BeNil())
		logger.Infof("ConnNum = %d ---> Receiver: opened session [%d] [%v]", connNum, sessId, session.Info())
		sessionMap[response.SessionID] = struct{}{}
		mu.Lock()
		*sessions = append(*sessions, session)
		mu.Unlock()
		wg.Add(1)
		go receiverSessionExchangeMsgs(connNum, sessId, session, wg)
	}
	logger.Infof("ConnNum = %d ---> Receiver: End receiving on master session", connNum)
}

func receiverSessionExchangeMsgs(connNum int, sessId int, session view.Session, wg *sync.WaitGroup) {
	defer wg.Done()
	payload := fmt.Sprintf("response-%d-0", sessId)
	logger.Infof("ConnNum = %d ---> Receiver: Send message [%s]", connNum, payload)
	Expect(session.Send([]byte(payload))).To(Succeed())
	logger.Infof("ConnNum = %d ---> Receiver: Sent message [%s]", connNum, payload)
	for response := range session.Receive() {
		if string(response.Payload) == "EOF" {
			logger.Infof("ConnNum = %d ---> Receiver: Received EOF on [%d]", connNum, sessId)
			return
		}
		elements := strings.Split(string(response.Payload), "-")
		Expect(elements).To(HaveLen(3))
		sessId, msgId := utils.MustGet(strconv.Atoi(elements[1])), utils.MustGet(strconv.Atoi(elements[2]))
		payload := fmt.Sprintf("response-%d-%d", sessId, msgId)
		logger.Infof("ConnNum = %d ---> Receiver: Send message [%s]", connNum, payload)
		Expect(session.Send([]byte(payload))).To(Succeed())
		logger.Infof("ConnNum = %d ---> Receiver: Sent message [%s]", connNum, payload)
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
	logger.Infof("Number of nodes: %d\n", numOfNodes)
	logger.Infof("Number of sessions per node: %d\n", numOfSessions)
	logger.Infof("Number of Msgs per session: %d\n", 2*numOfMsgs)
	logger.Infof("Average latency: %v\n", averageLatency)
	logger.Infof("Throughput: %f messages per second \n", throughput)
	logger.Infof("Total run time: %v\n", duration)
}
