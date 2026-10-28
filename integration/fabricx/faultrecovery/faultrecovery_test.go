/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package faultrecovery_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery"
	cviews "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/views"
	fxfaultrecovery "github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/faultrecovery"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate"
)

const dbPrefix = "frx"

// The specs share one network: each must leave it healthy, see faultrecovery.ShareNetwork.
var _ = Describe("Fault Recovery with Fabric-x", Ordered, ContinueOnFailure, Label(fsc.WebSocket), func() {
	s := integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		ii, err := integration.New(integration.FabricXFaultRecoveryPort.StartPortForNode(), "", fxfaultrecovery.Topology(dbPrefix)...)
		if err != nil {
			return nil, err
		}
		ii.RegisterPlatformFactory(nwofabricx.NewPlatformFactory())
		ii.EnableRaceDetector()
		ii.Generate()
		return ii, nil
	})
	faultrecovery.ShareNetwork(s)

	It("commits a transaction on a healthy network", func() {
		Expect(faultrecovery.SubmitAndWaitFinal(s.II, 1)).To(HaveLen(1))
	})

	faultrecovery.CrashSpecs(s)
	faultrecovery.ClientSpecs(s)
	faultrecovery.DependencySpecs(s, dbPrefix)

	// Pending: the all-in-one committer container also runs a mock orderer that keeps
	// blocks in memory only. After StartCommitter it restarts from the genesis block while
	// the sidecar resumes from the block it recovered from its database, so a transaction
	// ordered after the restart never commits. The spec runs once nwo can restart the
	// committer without the orderer (#2268).
	PIt("survives committer restart", func() {
		notification := fsc.Connection{Component: connstate.ComponentFabricxNotification}
		committer := nwofabricx.FxPlatform(s.II)
		tx := faultrecovery.Endorse(s.II, 1)

		// The committer container also runs the orderer, so the transaction is ordered
		// only once it is back. Bob's listener is registered while it is down, so it
		// waits on no stream and is requested on the one opened after the restart.
		committer.StopCommitter()
		s.II.EventuallyConnectionState("bob", notification, connstate.Reconnecting, time.Minute)
		final := make(chan error, 1)
		go func() {
			// Covers the wait below, StartCommitter, and ordering. The listener itself is
			// settled Unknown once the notification service's listenerTTL (2 min) passes.
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			_, err := s.II.Client("bob").CallViewWithContext(ctx, "finality", common.JSONMarshall(&cviews.Finality{TxID: tx.TxID}))
			final <- err
		}()
		// Nothing reports that the listener is registered. Registering first asks the
		// unreachable query service, which answers within its 10 s request timeout.
		time.Sleep(15 * time.Second)
		committer.StartCommitter()

		faultrecovery.Order(s.II, tx)
		// The finality view fails unless the listener is told Valid.
		Eventually(final, 5*time.Minute).Should(Receive(BeNil()))
		s.II.EventuallyConnectionState("bob", notification, connstate.Connected, time.Minute)
	})
})
