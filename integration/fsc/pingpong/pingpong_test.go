/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pingpong_test

import (
	"bytes"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/api"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/client/web"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
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
			responder.Stop()
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

			webClientConfig, err := web.NewConfigFromFSC("./testdata/fsc/nodes/initiator")
			Expect(err).NotTo(HaveOccurred())
			initiatorWebClient, err := web.NewClient(webClientConfig)
			Expect(err).NotTo(HaveOccurred())
			res, err := initiatorWebClient.CallView("init", bytes.NewBuffer([]byte("hi")).Bytes())
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

	Describe("Network-based Ping pong", func() {
		var (
			ii *integration.Infrastructure
		)

		AfterEach(func() {
			// Stop the ii
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

		It("generate artifacts & successful pingpong with Admin", func() {
			var err error
			// Create the integration ii
			ii, err = integration.Generate(StartPortWithAdmin(), true, pingpong.Topology()...)
			Expect(err).NotTo(HaveOccurred())
			// Start the integration ii
			ii.Start()
			time.Sleep(3 * time.Second)
			// Get an admin client for the fsc node labelled initiator
			initiatorAdmin := ii.Admin("initiator")
			// Initiate a view and check the output
			res, err := initiatorAdmin.CallView("init", nil)
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
