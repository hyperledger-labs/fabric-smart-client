/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pingpong_test

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/generic/pingpong"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
)

var _ = Describe("EndToEnd", func() {

	Describe("Node-based Ping pong", func() {

		It("successful pingpong", func() {
			// Init and Start fsc nodes
			initiator := node.NewFromConfPath("./testdata/fscnodes/initiator")
			Expect(initiator).NotTo(BeNil())

			responder := node.NewFromConfPath("./testdata/fscnodes/responder")
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

			initiator.Stop()
			responder.Stop()
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
			ii, err = integration.Generate(StartPort2(), pingpong.Topology()...)
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
			ii, err = integration.Load("./testdata", pingpong.Topology()...)
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

	})

})
