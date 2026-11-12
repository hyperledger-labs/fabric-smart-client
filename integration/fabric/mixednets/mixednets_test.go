/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mixednets_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/mixednets"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/twonets"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate"
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
		It("keeps committing on Fabric while the Fabric-x committer is down", func() {
			fabDelivery := fsc.Connection{Network: "fab", Component: connstate.ComponentDelivery}
			fxNotification := fsc.Connection{Network: "fx", Component: connstate.ComponentFabricxNotification}
			s.II.EventuallyConnectionState("alice", fabDelivery, connstate.Connected, time.Minute)
			s.II.EventuallyConnectionState("alice", fxNotification, connstate.Connected, time.Minute)

			committer := nwofabricx.FxPlatform(s.II)
			committer.StopCommitter()
			s.II.EventuallyConnectionState("alice", fxNotification, connstate.Reconnecting, time.Minute)

			twonets.EndorseAcross(s.II, []twonets.Endorsement{
				{Initiator: "alice", Endorser: "bob", Network: "fab", Namespace: "ns1"},
			})
			s.II.EventuallyConnectionState("alice", fabDelivery, connstate.Connected, time.Minute)

			committer.StartCommitter()
			s.II.EventuallyConnectionState("alice", fxNotification, connstate.Connected, faultrecovery.RecoveryTimeout)
			// Each node's gRPC connections to the committer reconnect on their own backoff, so
			// alice's notification stream being back says nothing of bob's connections. Bob's
			// endorsement reads from the committer's query service, whose connection state
			// changes only when a query answers, so it retries until bob's connections are back.
			twonets.EndorseAcrossEventually(s.II, []twonets.Endorsement{
				{Initiator: "bob", Endorser: "alice", Network: "fx", Namespace: "ns2"},
			}, faultrecovery.RecoveryTimeout)
		})
	})
})
