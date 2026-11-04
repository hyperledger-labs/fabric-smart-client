/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mixednets_test

import (
	. "github.com/onsi/ginkgo/v2"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/mixednets"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/twonets"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

var _ = Describe("EndToEnd", func() {
	Describe("A Fabric and a Fabric-x network", Label(fsc.WebSocket), func() {
		s := integration.NewTestSuite(func() (*integration.Infrastructure, error) {
			ii, err := integration.New(integration.FabricAndFabricXPort.StartPortForNode(), "", mixednets.Topology()...)
			if err != nil {
				return nil, err
			}
			ii.RegisterPlatformFactory(nwofabricx.NewPlatformFactory())
			ii.Generate()
			return ii, nil
		})
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("endorses a transaction received for the responder's non-default platform", func() {
			twonets.EndorseAcross(s.II, []twonets.Endorsement{
				{Initiator: "alice", Endorser: "bob", Network: "fab", Namespace: "ns1"},
				{Initiator: "bob", Endorser: "alice", Network: "fx", Namespace: "ns2"},
			})
		})
	})
})
