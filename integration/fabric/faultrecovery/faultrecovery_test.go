/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package faultrecovery_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery/views"
	fabricfaultrecovery "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/faultrecovery"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate"
)

const (
	dbPrefix = "fr"
	// bobPeer is the only peer bob's delivery attaches to: an FSC node is configured
	// with the peers of its own organization.
	bobPeer = "Org2_peer_0"

	commitRetries  = "fsc_fabric_core_generic_committer_commit_retries"
	commitFailures = "fsc_fabric_core_generic_committer_commit_failures"

	// postgresOutage labels the long Postgres outage specs, which run in a target of
	// their own.
	postgresOutage = "postgres-outage"
)

var (
	delivery = fsc.Connection{Component: connstate.ComponentDelivery}
	database = fsc.Connection{Component: connstate.ComponentDatabase}
)

func newInfrastructure() (*integration.Infrastructure, error) {
	return integration.Generate(
		integration.FabricFaultRecoveryPort.StartPortForNode(),
		integration.WithRaceDetection,
		fabricfaultrecovery.Topology(dbPrefix)...,
	)
}

// The specs share one network: each must leave it healthy, see faultrecovery.ShareNetwork.
var _ = Describe("Fault Recovery with Fabric", Ordered, ContinueOnFailure, Label(fsc.WebSocket), func() {
	s := integration.NewTestSuite(newInfrastructure)
	faultrecovery.ShareNetwork(s)

	It("commits a transaction on a healthy network", func() {
		Expect(faultrecovery.SubmitAndWaitFinal(s.II, 1)).To(HaveLen(1))
	})

	faultrecovery.CrashSpecs(s)
	faultrecovery.DependencySpecs(s, dbPrefix)

	It("survives delivery peer restart", func() {
		// Endorsing needs bob's peer for service discovery, so the tx is endorsed first.
		tx := faultrecovery.Endorse(s.II, 1)
		s.II.StopPeer(bobPeer)
		s.II.EventuallyConnectionState("bob", delivery, connstate.Reconnecting, time.Minute)

		// Alice orders through the orderers and learns finality from Org1's peer, so only
		// bob misses the block.
		faultrecovery.Order(s.II, tx)
		s.II.StartPeer(bobPeer)

		faultrecovery.WaitFinal(s.II, tx.TxID)
		s.II.EventuallyConnectionState("bob", delivery, connstate.Connected, time.Minute)
		// A full transaction proves that bob's peer endorses again before the next spec runs.
		Expect(faultrecovery.SubmitAndWaitFinal(s.II, 1)).To(HaveLen(1))
	})

	It("broadcasts via remaining orderers", func() {
		// Which orderer leads raft is not known, so the stopped one may be the leader:
		// OrderEventually rides out the election that follows.
		stopped := fabricfaultrecovery.Orderers[0]
		s.II.StopOrderer(stopped)
		tx := faultrecovery.Endorse(s.II, 1)
		faultrecovery.OrderEventually(s.II, tx)
		faultrecovery.WaitFinal(s.II, tx.TxID)

		for _, o := range fabricfaultrecovery.Orderers {
			if o != stopped {
				s.II.StopOrderer(o)
			}
		}
		for _, o := range fabricfaultrecovery.Orderers {
			s.II.StartOrderer(o)
		}
		tx = faultrecovery.Endorse(s.II, 2)
		faultrecovery.OrderEventually(s.II, tx)
		faultrecovery.WaitFinal(s.II, tx.TxID)
	})
})

// Its own Describe, so the shortened commit retry budget applies to these specs only. The
// specs share one network, see faultrecovery.ShareNetwork.
var _ = Describe("Fault Recovery with Fabric during a long Postgres outage", Ordered, ContinueOnFailure, Label(fsc.WebSocket, postgresOutage), func() {
	s := integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		ii, err := newInfrastructure()
		if err != nil {
			return nil, err
		}
		// A one-second commit retry budget, so a Postgres outage of a few seconds
		// already outlasts it.
		fabricfaultrecovery.SetCommitRetries(ii, "bob", 1, time.Second)
		return ii, nil
	})
	faultrecovery.ShareNetwork(s)

	It("resumes commits after Postgres restart", func() {
		db := faultrecovery.DBName(dbPrefix, "bob")

		// Endorsing writes to bob's database, so the txs are endorsed before the outage
		// and ordered during it.
		txs := make([]*views.Endorsed, 5)
		for i := range txs {
			txs[i] = faultrecovery.Endorse(s.II, i)
		}
		retries := faultrecovery.MetricSum(s.II, "bob", commitRetries)
		failures := faultrecovery.MetricSum(s.II, "bob", commitFailures)

		s.II.StopPostgres(db)
		s.II.EventuallyConnectionState("bob", database, connstate.Reconnecting, time.Minute)
		for _, tx := range txs {
			faultrecovery.Order(s.II, tx)
		}

		// Bob keeps retrying its block past the one-second budget instead of stopping
		// delivery.
		Eventually(func() float64 {
			return faultrecovery.MetricSum(s.II, "bob", commitRetries)
		}, time.Minute, time.Second).Should(BeNumerically(">", retries+1))
		Consistently(func(g Gomega) {
			g.Expect(faultrecovery.MetricSum(s.II, "bob", commitFailures)).To(Equal(failures))
			families, err := s.II.WebClient("bob").Metrics()
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(fsc.ParseConnectionState(families, delivery)).To(HaveEach(connstate.Connected))
		}, 10*time.Second, time.Second).Should(Succeed())

		s.II.StartPostgres(db)
		// While the database is down the committer backs off for up to a minute between
		// attempts, so the first commit after the restart can take that long.
		for _, tx := range txs {
			faultrecovery.WaitFinal(s.II, tx.TxID)
		}
		s.II.EventuallyConnectionState("bob", database, connstate.Connected, time.Minute)
		Expect(faultrecovery.MetricSum(s.II, "bob", commitFailures)).To(Equal(failures))
	})

	// Must stay the last spec: it kills bob's primary and points bob at the standby for good.
	It("fails over to a promoted Postgres standby", func() {
		db := faultrecovery.DataSource(s.II, "bob")
		primary, standby := db.Container, db.Container+"-standby"
		standbyPort := faultrecovery.StartStandby(primary, standby)

		// Bob lists both hosts and accepts only the one that takes writes.
		s.II.StopFSCNode("bob")
		faultrecovery.SetDataSource(s.II, "bob", "postgres://"+db.User+":"+db.Pass+"@127.0.0.1:"+db.Port+",127.0.0.1:"+standbyPort+
			"/"+db.DBName+"?target_session_attrs=read-write&connect_timeout=5&sslmode=disable")
		s.II.StartFSCNode("bob")

		// Endorsing writes to bob's database, so the txs are endorsed before the outage
		// and ordered during it.
		txs := make([]*views.Endorsed, 3)
		for i := range txs {
			txs[i] = faultrecovery.Endorse(s.II, i)
		}
		faultrecovery.WaitReplicated(primary, standby)
		retries := faultrecovery.MetricSum(s.II, "bob", commitRetries)
		failures := faultrecovery.MetricSum(s.II, "bob", commitFailures)

		faultrecovery.KillPostgres(primary)
		s.II.EventuallyConnectionState("bob", database, connstate.Reconnecting, time.Minute)
		for _, tx := range txs {
			faultrecovery.Order(s.II, tx)
		}

		// Bob keeps retrying its block past the one-second budget while no host
		// accepts writes.
		Eventually(func() float64 {
			return faultrecovery.MetricSum(s.II, "bob", commitRetries)
		}, time.Minute, time.Second).Should(BeNumerically(">", retries+1))
		Consistently(func() float64 {
			return faultrecovery.MetricSum(s.II, "bob", commitFailures)
		}, 10*time.Second, time.Second).Should(Equal(failures))

		faultrecovery.Promote(standby)
		for i, tx := range txs {
			Eventually(func(g Gomega) {
				res, err := faultrecovery.Query(s.II, "bob", tx.TxID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(res).To(Equal(&views.QueryResult{Status: fdriver.Valid, Value: i}))
			}, 2*time.Minute, time.Second).Should(Succeed(), "tx %d [%s] not settled on bob", i, tx.TxID)
		}
		s.II.EventuallyConnectionState("bob", database, connstate.Connected, time.Minute)
		Expect(faultrecovery.MetricSum(s.II, "bob", commitFailures)).To(Equal(failures))
	})
})
