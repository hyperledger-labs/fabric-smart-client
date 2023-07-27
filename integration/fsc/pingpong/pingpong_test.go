/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pingpong_test

import (
	"bytes"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/client"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
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

			webClientConfig, err := client.NewWebClientConfigFromFSC("./testdata/fsc/nodes/webclient")
			Expect(err).NotTo(HaveOccurred())
			initiatorWebClient, err := web.NewClient(webClientConfig)
			Expect(err).NotTo(HaveOccurred())
			res, err := initiatorWebClient.CallView("init", bytes.NewBuffer([]byte("hi")).Bytes())
			Expect(err).NotTo(HaveOccurred())
			Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
			version, err := initiatorWebClient.ServerVersion()
			Expect(err).NotTo(HaveOccurred())
			Expect(version).To(BeEquivalentTo("{\"CommitSHA\":\"development build\",\"Version\":\"latest\"}"))

			webClientConfig.TLSCert = ""
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

			initiatorWebClient := newWebClient("./testdata/fsc/nodes/webclient")
			stream, err := initiatorWebClient.StreamCallView("stream")
			Expect(err).NotTo(HaveOccurred())
			var s string
			Expect(stream.Recv(&s)).NotTo(HaveOccurred())
			Expect(s).To(BeEquivalentTo("hello"))
			Expect(stream.Send("ciao")).NotTo(HaveOccurred())
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

	Describe("Network-based Ping pong", func() {
		var (
			ii *integration.Infrastructure
		)

		AfterEach(func() {
			// Stop the ii
			ii.DeleteOnStop = false
			ii.Stop()
		})

		It("generate artifacts & successful pingpong", func() {
			var err error
			// Create the integration ii
			ii, err = integration.Generate(StartPortWithGeneration(), true, pingpong.Topology()...)
			Expect(err).NotTo(HaveOccurred())
			// Start the integration ii
			ii.Start()
			time.Sleep(3 * time.Second)
			// Get a client for the fsc node labelled initiator
			initiator := ii.Client("initiator")
			// Initiate a view and check the output
			res, err := initiator.CallView("init", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
		})

		It("load artifact & successful pingpong", func() {
			var err error
			// Create the integration ii
			ii, err = integration.Load(0, "./testdata", true, pingpong.Topology()...)
			Expect(err).NotTo(HaveOccurred())
			// Start the integration ii
			ii.Start()
			time.Sleep(3 * time.Second)
			// Get a client for the fsc node labelled initiator
			initiator := ii.Client("initiator")
			// Initiate a view and check the output
			res, err := initiator.CallView("init", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
		})

		It("load artifact & successful pingpong with stream", func() {
			var err error
			// Create the integration ii
			ii, err = integration.Load(0, "./testdata", true, pingpong.Topology()...)
			Expect(err).NotTo(HaveOccurred())
			// Start the integration ii
			ii.Start()
			time.Sleep(3 * time.Second)
			// Get a client for the fsc node labelled initiator
			initiator := ii.Client("initiator")
			// Initiate a view and check the output
			channel, err := initiator.StreamCallView("init", nil)
			Expect(err).NotTo(HaveOccurred())
			res, err := channel.Result()
			Expect(err).NotTo(HaveOccurred())
			Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
		})

		It("load artifact & successful stream", func() {
			var err error
			// Create the integration ii
			ii, err = integration.Load(0, "./testdata", true, pingpong.Topology()...)
			Expect(err).NotTo(HaveOccurred())
			// Start the integration ii
			ii.Start()
			time.Sleep(10 * time.Second)
			// Get a client for the fsc node labelled initiator
			initiator := ii.Client("initiator")
			// Initiate a view and check the output
			channel, err := initiator.StreamCallView("stream", nil)
			Expect(err).NotTo(HaveOccurred())
			var s string
			Expect(channel.Recv(&s)).NotTo(HaveOccurred())
			Expect(s).To(BeEquivalentTo("hello"))
			Expect(channel.Send("ciao")).NotTo(HaveOccurred())

			res, err := channel.Result()
			Expect(err).NotTo(HaveOccurred())
			Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
		})

		It("load artifact & successful stream with websocket", func() {
			var err error
			// Create the integration ii
			ii, err = integration.Load(0, "./testdata", true, pingpong.Topology()...)
			Expect(err).NotTo(HaveOccurred())
			// Start the integration ii
			ii.Start()
			time.Sleep(10 * time.Second)
			// Get a client for the fsc node labelled initiator
			initiator := newWebClient("./testdata/fsc/nodes/webclient")
			// Initiate a view and check the output
			channel, err := initiator.StreamCallView("stream")
			Expect(err).NotTo(HaveOccurred())
			var s string
			Expect(channel.Recv(&s)).NotTo(HaveOccurred())
			Expect(s).To(BeEquivalentTo("hello"))
			Expect(channel.Send("ciao")).NotTo(HaveOccurred())
		})

		It("load artifact & init clients & successful pingpong", func() {
			var err error
			// Create the integration ii
			ii, err = integration.Load(0, "./testdata", true, pingpong.Topology()...)
			Expect(err).NotTo(HaveOccurred())
			// Start the integration ii
			ii.Start()
			time.Sleep(3 * time.Second)

			// Use another ii to create clients
			iiClients, err := integration.Clients("./testdata", pingpong.Topology()...)
			Expect(err).NotTo(HaveOccurred())

			// Get a client for the fsc node labelled initiator
			initiator := iiClients.Client("initiator")
			// Initiate a view and check the output
			res, err := initiator.CallView("init", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
		})
	})

})

func newWebClient(confDir string) *web.Client {
	c, err := client.NewWebClientConfigFromFSC(confDir)
	Expect(err).NotTo(HaveOccurred())
	initiator, err := web.NewClient(c)
	Expect(err).NotTo(HaveOccurred())
	return initiator
}
