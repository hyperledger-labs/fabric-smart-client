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
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery/views"
	cviews "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/views"
	fxfaultrecovery "github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/faultrecovery"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate"
)

const (
	dbPrefix = "frx"
	// replication labels the specs on replicated nodes, which run in a target of their own.
	replication = "replication"
)

var (
	notification = fsc.Connection{Component: connstate.ComponentFabricxNotification}
	database     = fsc.Connection{Component: connstate.ComponentDatabase}

	// bobReplicas are the replica IDs of bob in faultrecovery.ReplicatedFSCTopology.
	bobReplicas = []string{"fsc.bob.0", "fsc.bob.1"}
)

func newInfrastructure(topologies ...api.Topology) (*integration.Infrastructure, error) {
	ii, err := integration.New(integration.FabricXFaultRecoveryPort.StartPortForNode(), "", topologies...)
	if err != nil {
		return nil, err
	}
	ii.RegisterPlatformFactory(nwofabricx.NewPlatformFactory())
	ii.EnableRaceDetector()
	ii.Generate()
	return ii, nil
}

// The specs share one network: each must leave it healthy, see faultrecovery.ShareNetwork.
var _ = Describe("Fault Recovery with Fabric-x", Ordered, ContinueOnFailure, Label(fsc.WebSocket), func() {
	s := integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		return newInfrastructure(fxfaultrecovery.Topology(dbPrefix)...)
	})
	faultrecovery.ShareNetwork(s)

	It("commits a transaction on a healthy network", func() {
		Expect(faultrecovery.SubmitAndWaitFinal(s.II, 1)).To(HaveLen(1))
	})

	faultrecovery.CrashSpecs(s)
	faultrecovery.ClientSpecs(s)
	faultrecovery.DependencySpecs(s, dbPrefix)

	It("survives committer restart", func() {
		committer := nwofabricx.FxPlatform(s.II)
		tx := faultrecovery.Endorse(s.II, 1)

		// Bob's listener is registered while the committer is down, so it waits on no
		// stream and is requested on the one opened after the restart. The transaction
		// is ordered after the restart, so the restarted sidecar has to deliver it.
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

// Bob's two replicas sit behind a load balancer; alice has two replicas without one. The
// specs share one network and must leave every replica running, see
// faultrecovery.ShareNetwork. A Fabric-x node keeps no ledger state, so faultrecovery.Query
// and the finality of a tx already final answer from the committer's query service on
// every replica. What a replica holds of its own is its notification stream and its
// database connection, which expectStreamedFinality and the connection states check.
var _ = Describe("Fault Recovery with Fabric-x and replicas", Ordered, ContinueOnFailure, Label(fsc.WebSocket, replication), func() {
	s := integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		return newInfrastructure(fxfaultrecovery.ReplicatedTopology(dbPrefix)...)
	})
	faultrecovery.ShareNetwork(s)

	It("commits through the load balancer", func() {
		Expect(faultrecovery.SubmitAndWaitFinal(s.II, 10)).To(HaveLen(10))
		// Alice multiplexes her sessions to bob over one connection, held by one replica.
		Expect(s.II.FscPlatform.LoadBalancer("bob").Connections()).To(HaveLen(1))
		expectStreamedFinality(s.II)
	})

	faultrecovery.FailoverSpecs(s, notification)

	It("settles txs ordered while Postgres is down on every replica", func() {
		db := faultrecovery.DBName(dbPrefix, "bob")
		// Endorsing writes to bob's database, so these txs are endorsed before the outage
		// and ordered during it, while bob's responder waits on their finality.
		txs := make([]*views.Endorsed, 3)
		for i := range txs {
			txs[i] = faultrecovery.Endorse(s.II, i)
		}

		s.II.StopPostgres(db)
		down := true
		// A failure during the outage would leave the database down for the next spec.
		DeferCleanup(func() {
			if down {
				s.II.StartPostgres(db)
			}
		})
		for _, r := range bobReplicas {
			s.II.EventuallyConnectionState(r, database, connstate.Reconnecting, time.Minute)
		}
		during := make([]string, len(txs))
		for i, tx := range txs {
			during[i] = faultrecovery.Order(s.II, tx)
		}
		s.II.StartPostgres(db)
		down = false
		for _, r := range bobReplicas {
			s.II.EventuallyConnectionState(r, database, connstate.Connected, time.Minute)
		}

		for _, r := range bobReplicas {
			for i, txID := range during {
				faultrecovery.ExpectSettled(s.II, r, txID, i)
			}
		}
		Expect(faultrecovery.SubmitAndWaitFinal(s.II, 3)).To(HaveLen(3))
	})
})

// expectStreamedFinality checks that the notification stream of every replica of bob
// answers a subscription: each replica's finality view registers its listener before a new
// tx is ordered, so the query service cannot settle it at registration, and a replica
// whose stream is down would hold the listener past the view's timeout.
func expectStreamedFinality(ii *integration.Infrastructure) {
	type result struct {
		replica string
		err     error
	}
	tx := faultrecovery.Endorse(ii, 1)
	final := make(chan result, len(bobReplicas))
	for _, r := range bobReplicas {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			_, err := ii.Client(r).CallViewWithContext(ctx, "finality", common.JSONMarshall(&cviews.Finality{TxID: tx.TxID}))
			final <- result{r, err}
		}()
	}
	// ponytail: nothing reports that a listener is registered, so this sleep covers the
	// registration's status query. A view that registers late still settles Valid from the
	// query service, so a short sleep weakens the check without failing it.
	time.Sleep(2 * time.Second)
	faultrecovery.Order(ii, tx)
	for range bobReplicas {
		var res result
		EventuallyWithOffset(1, final, 2*time.Minute).Should(Receive(&res))
		ExpectWithOffset(1, res.err).NotTo(HaveOccurred(), "finality of [%s] on [%s]", tx.TxID, res.replica)
	}
	for _, r := range bobReplicas {
		ii.EventuallyConnectionState(r, notification, connstate.Connected, time.Minute)
	}
}
