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
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	. "github.com/onsi/gomega"
)

const (
	numOfNodes    int = 3
	numOfSessions int = 1
	numOfMsgs     int = 1
)

type connection struct {
	senderNode   *Node
	receiverNode *Node
	senderId     int
	receiverId   int
}

type benchmarkMetrics struct {
	latency time.Duration
}

// MetricsContainer holds the metrics map and a mutex for synchronization.
type MetricsContainer struct {
	MetricsMap map[string]benchmarkMetrics
	Mutex      sync.Mutex
}

func createLibp2pNodes() []*Node {
	var nodes []*Node
	var resolver BootstrapNodeResolver
	var config *Config

	for iNode := 0; iNode < numOfNodes; iNode++ {
		nodeId := fmt.Sprintf("Node_%d", iNode)
		if iNode == 0 {
			config = GetConfig("initiator")
		} else {
			config = GetConfig("responder")
		}

		logger.Infof("Creating node=%s, address=%s", nodeId, config.ListenAddress)

		var service *Service
		var servicePkId view2.Identity
		if iNode == 0 {
			service, servicePkId = NewLibP2PCommService(config.Libp2pConfig(""), config.CertFile, nil)
			resolver = BootstrapNodeResolver{nodeID: servicePkId, nodeAddress: config.ListenAddress}
		} else {
			logger.Infof("%s: config.ListenAddress = %s", nodeId, config.ListenAddress)
			service, servicePkId = NewLibP2PCommService(config.Libp2pConfig(nodeId), config.CertFile, &resolver)
		}

		service.Start(context.Background())
		node := Node{
			commService: service,
			address:     config.ListenAddress,
			pkID:        []byte(servicePkId),
		}
		nodes = append(nodes, &node)
	}

	return nodes
}

func createWebsocketNodes() []*Node {
	router := routing.StaticIDRouter{}
	var configs []*Config
	var nodeIds []string

	for iNode := 0; iNode < numOfNodes; iNode++ {
		nodeId := fmt.Sprintf("Node_%d", iNode)
		nodeIds = append(nodeIds, nodeId)
		config := GetConfig("initiator")

		logger.Infof("Creating node=%s, address=%s", nodeId, config.ListenAddress)

		configs = append(configs, config)
		router[nodeId] = []host.PeerIPAddress{rest.ConvertAddress(config.ListenAddress)}
	}

	var nodes []*Node
	for iNode := 0; iNode < numOfNodes; iNode++ {
		service := NewWebsocketCommService(&router, configs[iNode].RestConfig())
		service.Start(context.Background())
		node := Node{
			commService: service,
			address:     configs[iNode].ListenAddress,
			pkID:        []byte(nodeIds[iNode]),
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
	for iSender := 0; iSender < numOfNodes; iSender++ {
		sender := nodes[iSender]
		for iReceiver := 0; iReceiver < numOfNodes; iReceiver++ {
			if iSender == iReceiver {
				continue
			}
			receiver := nodes[iReceiver]
			connections = append(connections, &connection{senderNode: sender, receiverNode: receiver, senderId: iSender, receiverId: iReceiver})
		}
	}

	return connections
}

// Main test:
// 1. Create a thread for each connection
// 2. wait for all threads to finish
// 3. close all connections
func testNodesExchange(nodes []*Node, connections []*connection, protocol string) {
	logger.Info("Starting the tests")
	metricsMap := MetricsContainer{
		MetricsMap: map[string]benchmarkMetrics{},
		Mutex:      sync.Mutex{},
	}

	mainwg := sync.WaitGroup{}
	logger.Info("mainwg: initializing")

	// start all receiving sockets
	receivingServiceWG := sync.WaitGroup{}
	for n := range nodes {
		// receiverServiceThread marks receivingServiceWG as done after the service is up.
		// It does NOT mark mainwg as done. mainwg is forwarded to the threads it creates (that handle the sessions)
		receivingServiceWG.Add(1)
		go receiverServiceThread(n, *nodes[n], &receivingServiceWG, &mainwg)
	}

	// wait for all services to start
	receivingServiceWG.Wait()

	startTime := time.Now()
	// start connections between each pair
	for pair := range connections {
		logger.Infof("Doing Pair = %d from Node %d to Node %d", pair, connections[pair].senderId, connections[pair].receiverId)
		for sessId := range numOfSessions {
			logger.Info("mainwg: increasing by 1")
			mainwg.Add(1)
			logger.Infof("starting thread for Pair = %d, Connection = %d", pair, sessId)
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

	logger.Info("All threads have finished. Stopping services")

	// Stop receiver services
	for n := range nodes {
		nodes[n].commService.Stop()
	}

	displayMetrics(endTime.Sub(startTime), metricsMap.MetricsMap, protocol)
}

func BenchmarkTestWebsocket(b *testing.B) {
	logger.Info("Starting websocket benchmark")
	RegisterTestingT(b)
	nodes := createWebsocketNodes()
	connections := connectNodesMesh(nodes)
	testNodesExchange(nodes, connections, "Websocket")
	logger.Info("Finished websocket benchmark")
}

func BenchmarkTestLibp2p(b *testing.B) {
	logger.Info("Started libp2p benchmark")
	RegisterTestingT(b)
	nodes := createLibp2pNodes()
	connections := connectNodesMesh(nodes)
	testNodesExchange(nodes, connections, "LibP2P")
	logger.Info("Finished libp2p benchmark")
}

// Test connections between a single sender-receiver pair.
// Gets a sender-node and a receiver-node then:
// 1. starts a single connection between them
// 2. pass messages on the connection
// 3. closes the connection
func singleConnectionThread(iPair int, iSession int, senderNode, receiverNode Node, metricsMap *MetricsContainer, mainwg *sync.WaitGroup) {
	defer mainwg.Done()
	defer logger.Infof("mainwg: singleConnectionThread Pair = %d, iSession = %d, decreasing by 1", iPair, iSession)
	logger.Infof("Pair = %d ---> Create new session # %d", iPair, iSession)
	senderSession, err := senderNode.commService.NewSession("", "", rest.ConvertAddress(receiverNode.address), receiverNode.pkID)
	Expect(err).ToNot(HaveOccurred())
	Expect(senderSession.Info().Endpoint).To(Equal(rest.ConvertAddress(receiverNode.address)))
	Expect(senderSession.Info().EndpointPKID).To(Equal(receiverNode.pkID.Bytes()))
	logger.Infof("Pair = %d ---> Created new session # %d [%v]", iPair, iSession, senderSession.Info())

	for msgId := range numOfMsgs {
		payload := fmt.Sprintf("request-%d-%d-%d", iPair, iSession, msgId)
		logger.Infof("Pair = %d, Connection = %d ---> Sender: sends message [%s]", iPair, iSession, payload)
		start := time.Now()
		Expect(senderSession.Send([]byte(payload))).To(Succeed())
		logger.Infof("Pair = %d ---> Sent on session # %d [%v]", iPair, iSession, senderSession.Info())
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

func receiverServiceThread(nodeNum int, receiverNode Node, servicewg *sync.WaitGroup, mainwg *sync.WaitGroup) {
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
		Expect(elements).To(HaveLen(4))
		pairId, sessId, msgId := utils.MustGet(strconv.Atoi(elements[1])), utils.MustGet(strconv.Atoi(elements[2])), utils.MustGet(strconv.Atoi(elements[3]))
		Expect(msgId).To(Equal(0))

		_, ok := sessionMap[response.SessionID]
		Expect(ok).To(BeFalse(), "we should not receive a second message on the master session [%s]", string(response.Payload))
		logger.Infof("Pair = %d, Node = %d ---> Receiver: open session [%d]", pairId, nodeNum, sessId)
		session, err := receiverNode.commService.NewSessionWithID(response.SessionID, "", response.FromEndpoint, response.FromPKID, nil, nil)
		Expect(err).To(BeNil())
		logger.Infof("Pair = %d, Node = %d ---> Receiver: opened session [%d] [%v]", pairId, nodeNum, sessId, session.Info())
		sessionMap[response.SessionID] = struct{}{}
		mainwg.Add(1)
		logger.Info("mainwg: increasing by 1")
		go receiverSessionThread(nodeNum, pairId, sessId, session, mainwg)
	}
	logger.Infof("Node = %d ---> Receiver: End receiving on master session", nodeNum)
}

func receiverSessionThread(nodeNum int, pairId int, sessId int, session view.Session, mainwg *sync.WaitGroup) {
	defer mainwg.Done()
	defer logger.Infof("mainwg: receiverSessionThread Node = %d, Pair = %d, Connection = %d, decreasing by 1", nodeNum, pairId, sessId)
	payload := fmt.Sprintf("response-%d-0", sessId)
	logger.Infof("Node = %d, Pair = %d, Connection = %d ---> Receiver: Send message [%s]", nodeNum, pairId, sessId, payload)
	Expect(session.Send([]byte(payload))).To(Succeed())
	for response := range session.Receive() {
		if string(response.Payload) == "EOF" {
			logger.Infof("Node = %d, Pair = %d, Connection = %d ---> Receiver: Received EOF", nodeNum, pairId, sessId)
			session.Close()
			return
		}
		elements := strings.Split(string(response.Payload), "-")
		Expect(elements).To(HaveLen(3))
		sessId, msgId := utils.MustGet(strconv.Atoi(elements[1])), utils.MustGet(strconv.Atoi(elements[2]))
		payload := fmt.Sprintf("response-%d-%d", sessId, msgId)
		logger.Infof("Node = %d, Pair = %d, Connection = %d ---> Receiver: Send message [%s]", nodeNum, pairId, sessId, payload)
		Expect(session.Send([]byte(payload))).To(Succeed())
		logger.Infof("Node = %d, Pair = %d, Connection = %d ---> Receiver: Sent message [%s]", nodeNum, pairId, sessId, payload)
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
	logger.Infof("Number of sessions per directed pair of nodes: %d\n", numOfSessions)
	logger.Infof("Number of Msgs (in each direction) per session: %d\n", numOfMsgs)
	logger.Infof("Average latency: %s\n", averageLatency)
	logger.Infof("Throughput: %s messages per second \n", throughput)
	logger.Infof("Total run time: %v\n", duration)
}
