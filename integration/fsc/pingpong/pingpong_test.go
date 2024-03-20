/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pingpong_test

import (
	"bytes"
	"strings"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong/mock"
	api2 "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
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
			initiator = node.NewFromConfPath("./testdata/fsc/nodes/initiator")
			Expect(initiator).NotTo(BeNil())

			responder = node.NewFromConfPath("./testdata/fsc/nodes/responder")
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

			webClientConfig, err := client.NewWebClientConfigFromFSC("./testdata/fsc/nodes/initiator")
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
			initiator = node.NewFromConfPath("./testdata/fsc/nodes/initiator")
			Expect(initiator).NotTo(BeNil())

			err := initiator.Start()
			Expect(err).NotTo(HaveOccurred())

			// Register views and view factories
			err = initiator.RegisterFactory("stream", &pingpong.StreamerViewFactory{})
			Expect(err).NotTo(HaveOccurred())

			time.Sleep(3 * time.Second)

			initiatorWebClient := newWebClient("./testdata/fsc/nodes/initiator")
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
			initiator = node.NewFromConfPath("./testdata/fsc/nodes/initiator")
			Expect(initiator).NotTo(BeNil())

			responder = node.NewFromConfPath("./testdata/fsc/nodes/responder")
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
		It("generate artifacts & successful pingpong", s.TestGenerateAndPingPong)
		It("load artifact & successful pingpong", s.TestLoadAndPingPong)
		It("load artifact & successful pingpong with stream", s.TestLoadAndPingPongStream)
		It("load artifact & successful stream", s.TestLoadAndStream)
		It("load artifact & successful stream with websocket", s.TestLoadAndStreamWebsocket)
		It("load artifact & init clients & successful pingpong", s.TestLoadInitPingPong)
	})

	Describe("Network-based Ping pong With Websockets", func() {
		s := TestSuite{commType: fsc.WebSocket}
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("generate artifacts & successful pingpong", s.TestGenerateAndPingPong)
		It("load artifact & successful pingpong", s.TestLoadAndPingPong)
		It("load artifact & successful pingpong with stream", s.TestLoadAndPingPongStream)
		It("load artifact & successful stream", s.TestLoadAndStream)
		It("load artifact & successful stream with websocket", s.TestLoadAndStreamWebsocket)
		It("load artifact & init clients & successful pingpong", s.TestLoadInitPingPong)
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

type TestSuite struct {
	commType fsc.P2PCommunicationType

	ii *integration.Infrastructure

	initiator api2.GRPCClient
}

func (s *TestSuite) TearDown() {
	// Stop the ii
	s.ii.DeleteOnStop = false
	s.ii.Stop()
}

func (s *TestSuite) Setup() {
	// Create the integration ii
	var err error
	if s.ii == nil {
		s.ii, err = integration.Generate(StartPortWithGeneration(), true, pingpong.Topology(s.commType)...)
	} else {
		s.ii, err = integration.Load(0, "./testdata", true, pingpong.Topology(s.commType)...)
	}
	Expect(err).NotTo(HaveOccurred())
	// Start the integration ii
	s.ii.Start()
	// Wait for network to start
	time.Sleep(3 * time.Second)
	// Get a client for the fsc node labelled initiator
	s.initiator = s.ii.Client("initiator")
}

func (s *TestSuite) TestGenerateAndPingPong() {
	// Initiate a view and check the output
	res, err := s.initiator.CallView("init", nil)
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
}

func (s *TestSuite) TestLoadAndPingPong() {
	// Initiate a view and check the output
	res, err := s.initiator.CallView("init", nil)
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
}

func (s *TestSuite) TestLoadAndPingPongStream() {
	// Initiate a view and check the output
	channel, err := s.initiator.StreamCallView("init", nil)
	Expect(err).NotTo(HaveOccurred())

	res, err := channel.Result()
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
}

func (s *TestSuite) TestLoadAndStream() {
	// Initiate a view and check the output
	channel, err := s.initiator.StreamCallView("stream", nil)
	Expect(err).NotTo(HaveOccurred())
	var str string
	Expect(channel.Recv(&str)).NotTo(HaveOccurred())
	Expect(str).To(BeEquivalentTo("hello"))
	Expect(channel.Send("ciao")).NotTo(HaveOccurred())

	res, err := channel.Result()
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
}

func (s *TestSuite) TestLoadAndStreamWebsocket() {
	time.Sleep(7 * time.Second)
	// Get a client for the fsc node labelled initiator
	initiator := s.ii.WebClient("initiator")
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

func (s *TestSuite) TestLoadInitPingPong() {
	// Use another ii to create clients
	iiClients, err := integration.Clients("./testdata", pingpong.Topology(s.commType)...)
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
	_, err := s.initiator.CallView("mockInit", common.JSONMarshall(&mock.Params{Mock: false}))
	Expect(err).To(HaveOccurred())
	Expect(strings.Contains(err.Error(), "expected mock pong, got pong")).To(BeTrue())

	// Init with mock=true, a success must happen
	res, err := s.initiator.CallView("mockInit", common.JSONMarshall(&mock.Params{Mock: true}))
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
