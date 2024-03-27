/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pingpong_test

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong/mock"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/client"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/api"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/client/web"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("EndToEnd", func() {

	Describe("Node-based Ping pong", func() {
		var (
			initiator api.FabricSmartClientNode
			responder api.FabricSmartClientNode
		)

		AfterEach(func() {
			// Stop the ii
			initiator.Stop()
			if responder != nil {
				responder.Stop()
			}
			time.Sleep(5 * time.Second)
		})

		It("successful pingpong based on REST API", func() {
			// Init and Start fsc nodes
			initiator = node.NewFromConfPath("./testdata/fsc/nodes/initiator.0")
			Expect(initiator).NotTo(BeNil())

			responder = node.NewFromConfPath("./testdata/fsc/nodes/responder.0")
			Expect(responder).NotTo(BeNil())

			err := initiator.Start()
			Expect(err).NotTo(HaveOccurred())
			err = responder.Start()
			Expect(err).NotTo(HaveOccurred())

			// Register views and view factories
			err = initiator.RegisterFactory("init", &pingpong.InitiatorViewFactory{})
			Expect(err).NotTo(HaveOccurred())
			responder.RegisterResponder(&pingpong.Responder{}, &pingpong.Initiator{})

			time.Sleep(3 * time.Second)

			webClientConfig, err := client.NewWebClientConfigFromFSC("./testdata/fsc/nodes/initiator.0")
			Expect(err).NotTo(HaveOccurred())
			initiatorWebClient, err := web.NewClient(webClientConfig)
			Expect(err).NotTo(HaveOccurred())
			res, err := initiatorWebClient.CallView("init", bytes.NewBuffer([]byte("hi")).Bytes())
			Expect(err).NotTo(HaveOccurred())
			Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
			version, err := initiatorWebClient.ServerVersion()
			Expect(err).NotTo(HaveOccurred())
			Expect(version).To(BeEquivalentTo("{\"CommitSHA\":\"development build\",\"Version\":\"latest\"}"))

			webClientConfig.TLSCertPath = ""
			initiatorWebClient, err = web.NewClient(webClientConfig)
			Expect(err).NotTo(HaveOccurred())
			_, err = initiatorWebClient.CallView("init", bytes.NewBuffer([]byte("hi")).Bytes())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("status code [401], status [401 Unauthorized]"))
			version, err = initiatorWebClient.ServerVersion()
			Expect(err).NotTo(HaveOccurred())
			Expect(version).To(BeEquivalentTo("{\"CommitSHA\":\"development build\",\"Version\":\"latest\"}"))
		})

		It("successful pingpong based on WebSocket", func() {
			// Init and Start fsc nodes
			initiator = node.NewFromConfPath("./testdata/fsc/nodes/initiator.0")
			Expect(initiator).NotTo(BeNil())

			err := initiator.Start()
			Expect(err).NotTo(HaveOccurred())

			// Register views and view factories
			err = initiator.RegisterFactory("stream", &pingpong.StreamerViewFactory{})
			Expect(err).NotTo(HaveOccurred())

			time.Sleep(3 * time.Second)

			initiatorWebClient := newWebClient("./testdata/fsc/nodes/initiator.0")
			stream, err := initiatorWebClient.StreamCallView("stream", nil)
			Expect(err).NotTo(HaveOccurred())
			var s string
			Expect(stream.Recv(&s)).NotTo(HaveOccurred())
			Expect(s).To(BeEquivalentTo("hello"))
			Expect(stream.Send("ciao")).NotTo(HaveOccurred())

			res, err := stream.Result()
			Expect(err).NotTo(HaveOccurred())
			Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
		})

		It("successful pingpong", func() {
			// Init and Start fsc nodes
			initiator = node.NewFromConfPath("./testdata/fsc/nodes/initiator.0")
			Expect(initiator).NotTo(BeNil())

			responder = node.NewFromConfPath("./testdata/fsc/nodes/responder.0")
			Expect(responder).NotTo(BeNil())

			err := initiator.Start()
			Expect(err).NotTo(HaveOccurred())
			err = responder.Start()
			Expect(err).NotTo(HaveOccurred())

			// Register views and view factories
			err = initiator.RegisterFactory("init", &pingpong.InitiatorViewFactory{})
			Expect(err).NotTo(HaveOccurred())
			responder.RegisterResponder(&pingpong.Responder{}, &pingpong.Initiator{})

			time.Sleep(3 * time.Second)
			// Initiate a view and check the output
			res, err := initiator.CallView("init", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
		})

	})

	Describe("Network-based Ping pong With LibP2P", func() {
		s := TestSuite{commType: fsc.LibP2P}
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("generate artifacts & successful pingpong", func() { s.TestGenerateAndPingPong("initiator") })
		It("load artifact & successful pingpong", func() { s.TestLoadAndPingPong("initiator") })
		It("load artifact & successful pingpong with stream", func() { s.TestLoadAndPingPongStream("initiator") })
		It("load artifact & successful stream", func() { s.TestLoadAndStream("initiator") })
		It("load artifact & successful stream with websocket", func() { s.TestLoadAndStreamWebsocket("initiator") })
		It("load artifact & init clients & successful pingpong", s.TestLoadInitPingPong)
	})

	Describe("Network-based Ping pong With Websockets", func() {
		s := TestSuite{commType: fsc.WebSocket}
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("generate artifacts & successful pingpong", func() { s.TestGenerateAndPingPong("initiator") })
		It("load artifact & successful pingpong", func() { s.TestLoadAndPingPong("initiator") })
		It("load artifact & successful pingpong with stream", func() { s.TestLoadAndPingPongStream("initiator") })
		It("load artifact & successful stream", func() { s.TestLoadAndStream("initiator") })
		It("load artifact & successful stream with websocket", func() { s.TestLoadAndStreamWebsocket("initiator") })
		It("load artifact & init clients & successful pingpong", s.TestLoadInitPingPong)
	})

	Describe("Network-based Ping pong With Websockets and replication", func() {
		s := TestSuite{
			commType:       fsc.WebSocket,
			alwaysGenerate: true,
			replicas: map[string]int{
				"initiator": 3,
			},
		}
		initiatorReplicas := GetFSCReplicaNames("initiator", s.replicas["initiator"])
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("generate artifacts & successful pingpong", func() { s.TestGenerateAndPingPong(initiatorReplicas...) })
		It("load artifact & successful pingpong", func() { s.TestLoadAndPingPong(initiatorReplicas...) })
		It("load artifact & successful pingpong with stream", func() { s.TestLoadAndPingPongStream(initiatorReplicas...) })
		It("load artifact & successful stream", func() { s.TestLoadAndStream(initiatorReplicas...) })
		It("load artifact & successful stream with websocket", func() { s.TestLoadAndStreamWebsocket(initiatorReplicas...) })
	})

	Describe("Network-based Mock Ping pong With LibP2P", func() {
		s := TestSuite{commType: fsc.LibP2P}
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("generate artifacts & successful mock pingpong", s.TestGenerateAndMockPingPong)
	})
	Describe("Network-based Mock Ping pong With Websockets", func() {
		s := TestSuite{commType: fsc.WebSocket}
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("generate artifacts & successful mock pingpong", s.TestGenerateAndMockPingPong)
	})
})

const testdataDir = "./testdata"

type TestSuite struct {
	commType       fsc.P2PCommunicationType
	replicas       map[string]int
	alwaysGenerate bool

	ii *integration.Infrastructure
}

func (s *TestSuite) TearDown() {
	// Stop the ii
	s.ii.DeleteOnStop = s.alwaysGenerate
	s.ii.Stop()
}

func (s *TestSuite) Setup() {
	// Create the integration ii
	var err error
	if s.ii == nil || s.alwaysGenerate {
		s.ii, err = integration.Generate(StartPortWithGeneration(), true, pingpong.Topology(s.commType, s.replicas)...)
	} else {
		s.ii, err = integration.Load(0, testdataDir, true, pingpong.Topology(s.commType, s.replicas)...)
	}
	Expect(err).NotTo(HaveOccurred())
	// Start the integration ii
	s.ii.Start()
	// Wait for network to start
	time.Sleep(3 * time.Second)
}

func (s *TestSuite) TestGenerateAndPingPong(clients ...string) {
	// Initiate a view and check the output
	for _, clientName := range clients {
		res, err := s.ii.Client(clientName).CallView("init", nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
	}
}

func (s *TestSuite) TestLoadAndPingPong(clients ...string) {
	// Initiate a view and check the output
	for _, clientName := range clients {
		res, err := s.ii.Client(clientName).CallView("init", nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
	}
}

func (s *TestSuite) TestLoadAndPingPongStream(clients ...string) {
	// Initiate a view and check the output
	for _, clientName := range clients {
		channel, err := s.ii.Client(clientName).StreamCallView("init", nil)
		Expect(err).NotTo(HaveOccurred())

		res, err := channel.Result()
		Expect(err).NotTo(HaveOccurred())
		Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
	}
}

func (s *TestSuite) TestLoadAndStream(clients ...string) {
	for _, clientName := range clients {
		channel, err := s.ii.Client(clientName).StreamCallView("stream", nil)
		Expect(err).NotTo(HaveOccurred())
		var str string
		Expect(channel.Recv(&str)).NotTo(HaveOccurred())
		Expect(str).To(BeEquivalentTo("hello"))
		Expect(channel.Send("ciao")).NotTo(HaveOccurred())

		res, err := channel.Result()
		Expect(err).NotTo(HaveOccurred())
		Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
	}
}

func (s *TestSuite) TestLoadAndStreamWebsocket(clients ...string) {
	time.Sleep(7 * time.Second)
	for _, clientName := range clients {
		// Get a client for the fsc node labelled initiator
		initiator := s.ii.WebClient(clientName)
		// Initiate a view and check the output
		channel, err := initiator.StreamCallView("stream", nil)
		Expect(err).NotTo(HaveOccurred())
		var str string
		Expect(channel.Recv(&str)).NotTo(HaveOccurred())
		Expect(str).To(BeEquivalentTo("hello"))
		Expect(channel.Send("ciao")).NotTo(HaveOccurred())

		res, err := channel.Result()
		Expect(err).NotTo(HaveOccurred())
		Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
	}
}

func (s *TestSuite) TestLoadInitPingPong() {
	// Use another ii to create clients
	iiClients, err := integration.Clients(testdataDir, pingpong.Topology(s.commType, s.replicas)...)
	Expect(err).NotTo(HaveOccurred())

	// Get a client for the fsc node labelled initiator
	initiator := iiClients.Client("initiator")
	// Initiate a view and check the output
	res, err := initiator.CallView("init", nil)
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
}

func (s *TestSuite) TestGenerateAndMockPingPong() {
	// Init with mock=false, a failure must happen
	_, err := s.ii.Client("initiator").CallView("mockInit", common.JSONMarshall(&mock.Params{Mock: false}))
	Expect(err).To(HaveOccurred())
	Expect(strings.Contains(err.Error(), "expected mock pong, got pong")).To(BeTrue())

	// Init with mock=true, a success must happen
	res, err := s.ii.Client("initiator").CallView("mockInit", common.JSONMarshall(&mock.Params{Mock: true}))
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
}

func newWebClient(confDir string) *web.Client {
	c, err := client.NewWebClientConfigFromFSC(confDir)
	Expect(err).NotTo(HaveOccurred())
	initiator, err := web.NewClient(c)
	Expect(err).NotTo(HaveOccurred())
	return initiator
}

func GetFSCReplicaNames(nodeName string, replicationFactor int) []string {
	result := make([]string, replicationFactor)
	for i := 0; i < replicationFactor; i++ {
		result[i] = fmt.Sprintf("fsc.%s.%d", nodeName, i)
	}
	return result
}
