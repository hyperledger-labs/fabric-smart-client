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
	numOfNodes    int = 10
	numOfSessions int = 10
	numOfMsgs     int = 10
)

type connection struct {
	senderNode   *Node
	receiverNode *Node
}

type benchmarkMetrics struct {
	latency time.Duration
}

// MetricsContainer holds the metrics map and a mutex for synchronization.
type MetricsContainer struct {
	MetricsMap map[string]benchmarkMetrics
	Mutex      sync.Mutex
}

func createWebsocketNodes() []*Node {
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
		service := NewWebsocketCommService(&router, configs[nodeNum])
		service.Start(context.Background())
		node := Node{
			commService: service,
			address:     configs[nodeNum].ListenAddress,
			pkID:        []byte(nodeIds[nodeNum]),
		}
		nodes = append(nodes, &node)
	}

	return nodes
}

// Connect nodes in a mesh topology
// e.g., for 3 nodes (A, B, C)
// A -> B
// A -> C
// B -> A
// B -> C
// C -> A
// C -> B
func connectNodesMesh(nodes []*Node) []*connection {
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

// Main test:
// 1. Create a thread for each connection
// 2. wait for all threads to finish
// 3. close all connections
func testNodesExchange(nodes []*Node, connections []*connection, protocol string) {
	metricsMap := MetricsContainer{
		MetricsMap: map[string]benchmarkMetrics{},
		Mutex:      sync.Mutex{},
	}

	mainwg := sync.WaitGroup{}

	// start all receiving sockets
	receivingServiceWG := sync.WaitGroup{}
	for n := range nodes {
		receivingServiceWG.Add(1)
		go receiverServiceThread(n, *nodes[n], &receivingServiceWG, &mainwg)
	}

	// wait for services to start
	receivingServiceWG.Wait()

	startTime := time.Now()
	// start connections between each pair
	for pair := range connections {
		for sessId := range numOfSessions {
			mainwg.Add(1)
			go singleConnectionThread(pair,
				sessId,
				*connections[pair].senderNode,
				*connections[pair].receiverNode,
				&metricsMap,
				&mainwg)
		}
	}
	mainwg.Wait()
	endTime := time.Now()

	// Stop receiver services
	for n := range nodes {
		nodes[n].commService.Stop()
	}

	displayMetrics(endTime.Sub(startTime), metricsMap.MetricsMap, protocol)
}

func BenchmarkTestWebsocket(b *testing.B) {
	RegisterTestingT(b)
	nodes := createWebsocketNodes()
	connections := connectNodesMesh(nodes)
	testNodesExchange(nodes, connections, "Websocket")
}

// func BenchmarkTestLibp2p(b *testing.B) {
// 	RegisterTestingT(b)
// 	testNodesExchange(createLibp2pNodes(), "LipP2P")
// }

// Test connections between a single sender-receiver pain.
// Gets a sender-node and a receiver-node then:
// 1. starts a single connection between them
// 2. pass messages on the connection
// 3. closes the connection
func singleConnectionThread(iPair int, iSession int, senderNode, receiverNode Node, metricsMap *MetricsContainer, wg *sync.WaitGroup) {
	defer wg.Done()
	logger.Infof("Pair = %d ---> Create new session # %d", iPair, iSession)
	senderSession, err := senderNode.commService.NewSession("", "", rest.ConvertAddress(receiverNode.address), receiverNode.pkID)
	Expect(err).ToNot(HaveOccurred())
	Expect(senderSession.Info().Endpoint).To(Equal(rest.ConvertAddress(receiverNode.address)))
	Expect(senderSession.Info().EndpointPKID).To(Equal(receiverNode.pkID.Bytes()))
	logger.Infof("Pair = %d ---> Created new session # %d [%v]", iPair, iSession, senderSession.Info())

	for msgId := range numOfMsgs {
		payload := fmt.Sprintf("request-%d-%d", iSession, msgId)
		logger.Infof("Pair = %d, Connection = %d ---> Sender: sends message [%s]", iPair, iSession, payload)
		start := time.Now()
		Expect(senderSession.Send([]byte(payload))).To(Succeed())
		logger.Infof("Pair = %d, Connection = %d ---> Sender: sent message [%s]", iPair, iSession, payload)
		response := <-senderSession.Receive()
		end := time.Now()
		elapsed := end.Sub(start)
		Expect(response).ToNot(BeNil())
		Expect(string(response.Payload)).To(Equal(fmt.Sprintf("response-%d-%d", iSession, msgId)))
		logger.Infof("Pair = %d, Connection = %d ---> Sender: received message: [%s]", iPair, iSession, string(response.Payload))

		metricsMap.Mutex.Lock()
		key := strconv.Itoa(iPair) + senderSession.Info().ID + strconv.Itoa(iSession) + strconv.Itoa(msgId)
		_, ok := metricsMap.MetricsMap[key]
		if ok {
			Expect(ok).To(BeFalse(), "Metrics map keys should be unique")
		}
		metricsMap.MetricsMap[key] = benchmarkMetrics{latency: elapsed}
		metricsMap.Mutex.Unlock()
	}
	logger.Infof("Pair = %d, Connection = %d ---> Sender: sending EOF", iPair, iSession)
	Expect(senderSession.Send([]byte("EOF"))).To(Succeed())
	senderSession.Close()
}

func receiverServiceThread(nodeNum int, receiverNode Node, servicewg *sync.WaitGroup, wg *sync.WaitGroup) {
	receiverMasterSession, err := receiverNode.commService.MasterSession()
	Expect(err).ToNot(HaveOccurred())
	servicewg.Done()

	sessionMap := map[string]struct{}{}

	logger.Infof("Node = %d ---> Receiver: start receiving on master session", nodeNum)
	for response := range receiverMasterSession.Receive() {
		Expect(response).ToNot(BeNil())
		logger.Infof("Node = %d ---> Receiver: received message [%s]", nodeNum, string(response.Payload))
		Expect(string(response.Payload)).To(ContainSubstring("request"))
		elements := strings.Split(string(response.Payload), "-")
		Expect(elements).To(HaveLen(3))
		sessId, msgId := utils.MustGet(strconv.Atoi(elements[1])), utils.MustGet(strconv.Atoi(elements[2]))
		Expect(msgId).To(Equal(0))

		_, ok := sessionMap[response.SessionID]
		Expect(ok).To(BeFalse(), "we should not receive a second message on the master session [%s]", string(response.Payload))
		logger.Infof("Node = %d ---> Receiver: open session [%d]", nodeNum, sessId)
		session, err := receiverNode.commService.NewSessionWithID(response.SessionID, "", response.FromEndpoint, response.FromPKID, nil, nil)
		Expect(err).To(BeNil())
		logger.Infof("Node = %d ---> Receiver: opened session [%d] [%v]", nodeNum, sessId, session.Info())
		sessionMap[response.SessionID] = struct{}{}
		wg.Add(1)
		go receiverSessionThread(nodeNum, sessId, session, wg)
	}
	logger.Infof("Node = %d ---> Receiver: End receiving on master session", nodeNum)
}

func receiverSessionThread(nodeNum int, sessId int, session view.Session, wg *sync.WaitGroup) {
	defer wg.Done()
	payload := fmt.Sprintf("response-%d-0", sessId)
	logger.Infof("Node = %d, Connection = %d ---> Receiver: Send message [%s]", nodeNum, sessId, payload)
	Expect(session.Send([]byte(payload))).To(Succeed())
	logger.Infof("Node = %d, Connection = %d ---> Receiver: Sent message [%s]", nodeNum, sessId)
	for response := range session.Receive() {
		if string(response.Payload) == "EOF" {
			logger.Infof("Node = %d, Connection = %d ---> Receiver: Received EOF", nodeNum, sessId)
			session.Close()
			return
		}
		elements := strings.Split(string(response.Payload), "-")
		Expect(elements).To(HaveLen(3))
		sessId, msgId := utils.MustGet(strconv.Atoi(elements[1])), utils.MustGet(strconv.Atoi(elements[2]))
		payload := fmt.Sprintf("response-%d-%d", sessId, msgId)
		logger.Infof("Node = %d, Connection = %d ---> Receiver: Send message [%s]", nodeNum, sessId, payload)
		Expect(session.Send([]byte(payload))).To(Succeed())
		logger.Infof("Node = %d, Connection = %d ---> Receiver: Sent message [%s]", nodeNum, sessId, payload)
	}
	panic("Finished before receiving EOF")
}

func displayMetrics(duration time.Duration, metricsMap map[string]benchmarkMetrics, protocol string) {
	var latency time.Duration = 0

	for _, metric := range metricsMap {
		latency = latency + metric.latency
	}

	averageLatency := ""
	if time.Duration(len(metricsMap)) == 0 {
		averageLatency = "Inf"
	} else {
		averageLatency = fmt.Sprintf("%v", latency/time.Duration(len(metricsMap)))
	}
	throughput := ""
	if duration.Seconds() == 0 {
		throughput = "Inf"
	} else {
		throughput = fmt.Sprintf("%f", float64(2*numOfMsgs)/float64(duration.Seconds()))
	}

	logger.Infof("============================= %s Metrics =============================\n", protocol)
	logger.Infof("Number of nodes: %d\n", numOfNodes)
	logger.Infof("Number of sessions per node: %d\n", numOfSessions)
	logger.Infof("Number of Msgs per session: %d\n", 2*numOfMsgs)
	logger.Infof("Average latency: %s\n", averageLatency)
	logger.Infof("Throughput: %s messages per second \n", throughput)
	logger.Infof("Total run time: %v\n", duration)
}
