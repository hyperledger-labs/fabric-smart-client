/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package faultrecovery_test

import (
	"context"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/configupdate"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery/views"
	cviews "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/views"
	fabricfaultrecovery "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/faultrecovery"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	fabricsdk "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
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
	// replication labels the specs on replicated nodes, which run in a target of their own.
	replication = "replication"
	// partition labels the specs that cut links silently through fault proxies, which run
	// in a target of their own.
	partition = "partition"
)

var (
	delivery = fsc.Connection{Component: connstate.ComponentDelivery}
	database = fsc.Connection{Component: connstate.ComponentDatabase}

	// bobReplicas are the replica IDs of bob in faultrecovery.ReplicatedFSCTopology.
	bobReplicas = []string{"fsc.bob.0", "fsc.bob.1"}
)

func newInfrastructure() (*integration.Infrastructure, error) {
	return newInfrastructureWith(&fabricsdk.SDK{})
}

func newInfrastructureWith(sdk node.SDK) (*integration.Infrastructure, error) {
	return integration.Generate(
		integration.FabricFaultRecoveryPort.StartPortForNode(),
		integration.WithRaceDetection,
		fabricfaultrecovery.Topology(dbPrefix, sdk)...,
	)
}

// The specs share one network: each must leave it healthy, see faultrecovery.ShareNetwork.
var _ = Describe("Fault Recovery with Fabric", Ordered, ContinueOnFailure, Label(fsc.WebSocket), func() {
	s := integration.NewTestSuite(newInfrastructure)
	faultrecovery.ShareNetwork(s)

	It("commits a transaction on a healthy network", func() {
		Expect(faultrecovery.SubmitAndWaitFinal(s.II, 1)).To(HaveLen(1))
	})

	It("reads a committed transaction back through the ledger API", func() {
		txID := faultrecovery.SubmitAndWaitFinal(s.II, 1)[0]
		unknownTxID := "unknown-" + txID
		res := faultrecovery.LedgerOf(s.II, "alice", txID, unknownTxID)
		Expect(res.ValidationCode).To(Equal(int32(peer.TxValidationCode_VALID)))
		Expect(res.BlockHasTx).To(BeTrue())
		Expect(res.LedgerHeight).To(BeNumerically(">", res.BlockNumber))
		// Callers such as Panurus tell a missing transaction apart by this message.
		Expect(res.UnknownTxErr).To(ContainSubstring("no such transaction ID [" + unknownTxID + "]"))
	})

	faultrecovery.CrashSpecs(s)
	faultrecovery.ClientSpecs(s)
	faultrecovery.DependencySpecs(s, dbPrefix)

	It("applies a config update made while down", func() {
		original := configupdate.BatchTimeout(s.II)
		before := configupdate.ConfigSequenceOn(Default, s.II, "bob")

		// Bob endorses the tx, then misses the config block and the later block that
		// commits the tx. A small BatchTimeout change keeps this spec fast.
		tx := faultrecovery.Endorse(s.II, 1)
		s.II.StopFSCNode("bob")
		configupdate.SetBatchTimeout(s.II, original+500*time.Millisecond)
		faultrecovery.Order(s.II, tx)
		s.II.StartFSCNode("bob")

		// Bob receives the config block from delivery, then commits the block after it.
		// The sequence moves once the membership service is updated, before ordering is
		// reconfigured; a failed reconfiguration makes bob retry the config block, which
		// holds back the tx's block, so WaitFinal on bob catches it.
		expectConfigSequence(s.II, before+1, "bob", "alice")
		faultrecovery.WaitFinal(s.II, tx.TxID)
		// A new tx needs bob to endorse it on the new configuration.
		Expect(faultrecovery.SubmitAndWaitFinal(s.II, 1)).To(HaveLen(1))

		// Leave the shared network's BatchTimeout as this spec found it.
		configupdate.SetBatchTimeout(s.II, original)
		expectConfigSequence(s.II, before+2, "alice", "bob")
	})

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

// Its own Describe, so the namespace registration that lets a wiped node rebuild does
// not apply to the specs above, where it would let bob commit a tx whose Busy record
// was lost.
var _ = Describe("Fault Recovery with Fabric on a node that rebuilds its vault", Ordered, ContinueOnFailure, Label(fsc.WebSocket), func() {
	s := integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		return newInfrastructureWith(&fabricfaultrecovery.SDK{})
	})
	faultrecovery.ShareNetwork(s)

	It("rebuilds a node whose vault was wiped", func() {
		txIDs := faultrecovery.SubmitAndWaitFinal(s.II, 10)
		s.II.StopFSCNode("bob")
		faultrecovery.WipePostgres(faultrecovery.DBName(dbPrefix, "bob"))
		s.II.StartFSCNode("bob")

		// Bob replays the channel from the genesis block and commits these txs again
		// because their namespace is registered, see fabricfaultrecovery.SDK. The
		// replay takes about a second, well within RecoveryTimeout.
		for i, txID := range txIDs {
			Eventually(func(g Gomega) {
				res, err := faultrecovery.Query(s.II, "bob", txID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(res).To(Equal(&views.QueryResult{Status: fdriver.Valid, Value: i}))
			}, faultrecovery.RecoveryTimeout, time.Second).Should(Succeed(), "tx %d [%s] not rebuilt on bob", i, txID)
		}
		Expect(faultrecovery.SubmitAndWaitFinal(s.II, 1)).To(HaveLen(1))
	})
})

// Bob's two replicas sit behind a load balancer; alice has two replicas without one. The
// specs share one network and must leave every replica running, see
// faultrecovery.ShareNetwork.
var _ = Describe("Fault Recovery with Fabric and replicas", Ordered, ContinueOnFailure, Label(fsc.WebSocket, replication), func() {
	s := integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		return integration.Generate(
			integration.FabricFaultRecoveryPort.StartPortForNode(),
			integration.WithRaceDetection,
			fabricfaultrecovery.ReplicatedTopology(dbPrefix, &fabricsdk.SDK{})...,
		)
	})
	faultrecovery.ShareNetwork(s)

	It("commits through the load balancer", func() {
		Expect(faultrecovery.SubmitAndWaitFinal(s.II, 10)).To(HaveLen(10))
		// Alice multiplexes her sessions to bob over one connection, held by one replica.
		Expect(s.II.FscPlatform.LoadBalancer("bob").Connections()).To(HaveLen(1))
	})

	It("settles a tx on the surviving replica", func() {
		// The bob replica holding alice's connection endorses the tx and stores it in the
		// shared vault.
		tx := faultrecovery.Endorse(s.II, 1)
		endorser, others := faultrecovery.ConnectedReplica(s.II, "bob")
		s.II.KillFSCNode(endorser.UniqueName)
		faultrecovery.Order(s.II, tx)

		for _, r := range others {
			faultrecovery.ExpectSettled(s.II, r.ID(), tx.TxID, 1)
		}
		s.II.StartFSCNode(endorser.UniqueName)
		faultrecovery.ExpectSettled(s.II, endorser.ID(), tx.TxID, 1)
	})

	faultrecovery.FailoverSpecs(s, delivery)

	It("keeps committing when one replica is killed", func() {
		// Both replicas commit every block to the shared vault. The killed replica holds no
		// session from alice, so it dies as a committer and no tx may fail. The first tx
		// opens alice's connection, which ConnectedReplica looks for.
		Expect(faultrecovery.SubmitAndWaitFinal(s.II, 1)).To(HaveLen(1))
		held, others := faultrecovery.ConnectedReplica(s.II, "bob")
		Expect(others).To(HaveLen(1))
		idle := others[0]
		var txIDs []string
		for i := range 10 {
			if i == 3 {
				s.II.KillFSCNode(idle.UniqueName)
			}
			txIDs = append(txIDs, faultrecovery.Order(s.II, faultrecovery.Endorse(s.II, i)))
		}
		// Not faultrecovery.WaitFinal: it asks Client("bob"), which may be the dead replica.
		for i, txID := range txIDs {
			faultrecovery.ExpectSettled(s.II, held.ID(), txID, i)
		}

		// The replicas share one vault, so the Query shows that the restarted replica
		// serves views again; its connected delivery shows that it commits again.
		s.II.StartFSCNode(idle.UniqueName)
		last := len(txIDs) - 1
		faultrecovery.ExpectSettled(s.II, idle.ID(), txIDs[last], last)
		for _, r := range []string{held.ID(), idle.ID()} {
			s.II.EventuallyConnectionState(r, delivery, connstate.Connected, time.Minute)
		}
	})

	// Guards #2253: each replica holds its own membership service, so each must apply a
	// config block, also one that another replica already recorded in the shared vault.
	It("applies a config update another replica recorded", func() {
		replicas := []string{"fsc.alice.0", "fsc.alice.1", "fsc.bob.0", "fsc.bob.1"}
		original := configupdate.BatchTimeout(s.II)
		before := configupdate.ConfigSequenceOn(Default, s.II, replicas[0])

		// Live replicas receive the config block at about the same time and both apply
		// it, so alice.1 is frozen until alice.0 has recorded it. No view runs on alice
		// meanwhile, so the pause blocks nothing else.
		s.II.PauseFSCNode("alice.1")
		DeferCleanup(s.II.ResumeFSCNode, "alice.1")
		configupdate.SetBatchTimeout(s.II, original+500*time.Millisecond)
		expectConfigSequence(s.II, before+1, "fsc.alice.0")
		s.II.ResumeFSCNode("alice.1")
		expectConfigSequence(s.II, before+1, replicas...)
		Expect(faultrecovery.SubmitAndWaitFinal(s.II, 1)).To(HaveLen(1))

		// Leave the shared network's BatchTimeout as this spec found it.
		configupdate.SetBatchTimeout(s.II, original)
		expectConfigSequence(s.II, before+2, replicas...)
	})

	It("closes the responder session when an initiator replica dies", func() {
		// Either bob replica may hold alice.1's session, so the gauges are summed over both.
		Eventually(func() float64 {
			return bobSum(s.II, faultrecovery.ViewContexts)
		}, time.Minute, time.Second).Should(BeZero())
		// A session left over from an earlier spec closes on its own sessionStreamGrace (5 s)
		// after its stream died, so the baseline is a count that holds for 10 s.
		var sessions float64
		Eventually(func(g Gomega) {
			sessions = bobSum(s.II, faultrecovery.Sessions)
			g.Consistently(func() float64 {
				return bobSum(s.II, faultrecovery.Sessions)
			}, 10*time.Second, time.Second).Should(Equal(sessions))
		}, time.Minute, time.Second).Should(Succeed())

		echo := make(chan error, 1)
		go func() {
			defer GinkgoRecover()
			echo <- faultrecovery.AwaitEcho(s.II, s.II.Client("fsc.alice.1"))
		}()
		Eventually(func() float64 {
			return bobSum(s.II, faultrecovery.ViewContexts)
		}, time.Minute, 100*time.Millisecond).Should(BeNumerically(">", 0))
		// Bob registers the responder session before it creates the responder's context.
		Expect(bobSum(s.II, faultrecovery.Sessions)).To(Equal(sessions + 1))
		s.II.KillFSCNode("alice.1")
		DeferCleanup(s.II.StartFSCNode, "alice.1")
		Eventually(echo, time.Minute).Should(Receive(HaveOccurred()))

		Eventually(func(g Gomega) {
			g.Expect(bobSum(s.II, faultrecovery.ViewContexts)).To(BeZero())
			g.Expect(bobSum(s.II, faultrecovery.Sessions)).To(Equal(sessions))
		}, faultrecovery.SessionCloseTimeout, time.Second).Should(Succeed())
		Expect(faultrecovery.SubmitAndWaitFinalFrom(s.II, "fsc.alice.0", 1)).To(HaveLen(1))
	})

	It("resumes commits on every replica after Postgres restart", func() {
		db := faultrecovery.DBName(dbPrefix, "bob")
		// The outage stays below the default commit retry budget of about a minute (5
		// retries, 10 s apart), and a lost connection does not use up the budget anyway.
		const outage = 20 * time.Second

		before := faultrecovery.SubmitAndWaitFinal(s.II, 3)
		// Endorsing writes to bob's database, so these txs are endorsed before the outage
		// and ordered during it, and both replicas retry their blocks against it.
		txs := make([]*views.Endorsed, 3)
		for i := range txs {
			txs[i] = faultrecovery.Endorse(s.II, i)
		}
		retries, failures := map[string]float64{}, map[string]float64{}
		for _, r := range bobReplicas {
			retries[r] = faultrecovery.MetricSum(s.II, r, commitRetries)
			failures[r] = faultrecovery.MetricSum(s.II, r, commitFailures)
		}
		expectHealthyCommits(s.II, failures)

		stopped := time.Now()
		s.II.StopPostgres(db)
		for _, r := range bobReplicas {
			s.II.EventuallyConnectionState(r, database, connstate.Reconnecting, time.Minute)
		}
		during := make([]string, len(txs))
		for i, tx := range txs {
			during[i] = faultrecovery.Order(s.II, tx)
		}
		for _, r := range bobReplicas {
			Eventually(func() float64 {
				return faultrecovery.MetricSum(s.II, r, commitRetries)
			}, time.Minute, time.Second).Should(BeNumerically(">", retries[r]), "commit retries on [%s]", r)
		}
		// The sleep is the rest of the outage: nothing signals that it has lasted long enough.
		time.Sleep(time.Until(stopped.Add(outage)))

		s.II.StartPostgres(db)
		for _, r := range bobReplicas {
			s.II.EventuallyConnectionState(r, database, connstate.Connected, time.Minute)
		}
		after := faultrecovery.SubmitAndWaitFinal(s.II, 3)

		for _, r := range bobReplicas {
			for _, txIDs := range [][]string{before, during, after} {
				for i, txID := range txIDs {
					faultrecovery.ExpectSettled(s.II, r, txID, i)
				}
			}
		}
		// A stopped delivery stays stopped and the failure counter only grows, so a check
		// after the outage also catches a failure during it.
		expectHealthyCommits(s.II, failures)
	})
})

// expectHealthyCommits checks that no replica of bob has stopped its delivery or recorded
// a commit failure beyond failures, its count per replica ID.
func expectHealthyCommits(ii *integration.Infrastructure, failures map[string]float64) {
	for _, r := range bobReplicas {
		ExpectWithOffset(1, faultrecovery.MetricSum(ii, r, commitFailures)).To(Equal(failures[r]), "commit failures on [%s]", r)
		families, err := ii.WebClient(r).Metrics()
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		ExpectWithOffset(1, fsc.ParseConnectionState(families, delivery)).NotTo(ContainElement(connstate.Stopped), "delivery on [%s]", r)
	}
}

// bobSum returns the sum of the counter or gauge name over bob's replicas.
func bobSum(ii *integration.Infrastructure, name string) float64 {
	var sum float64
	for _, r := range bobReplicas {
		sum += faultrecovery.MetricSum(ii, r, name)
	}
	return sum
}

// expectConfigSequence waits until every node holds channel configuration sequence want.
// A CONFIG block reaches a node asynchronously over delivery, so this polls.
func expectConfigSequence(ii *integration.Infrastructure, want int, nodes ...string) {
	for _, node := range nodes {
		EventuallyWithOffset(1, func(g Gomega) int {
			return configupdate.ConfigSequenceOn(g, ii, node)
		}, time.Minute, time.Second).Should(Equal(want), "config sequence on [%s]", node)
	}
}

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
		// IsFinal on bob gives up on a tx that is not final after 15 s instead of the 300 s
		// default, so a spec sees the error for a tx that is never ordered. It also bounds
		// bob's responder, which then stops waiting for a tx delayed by an outage before
		// the tx commits; no spec here asserts on the responder.
		fabricfaultrecovery.SetFinalityWaitTimeout(ii, "bob", 15*time.Second)
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

	It("reports a never-ordered tx as busy after a restart", func() {
		// Bob endorses the tx and holds it as Busy; alice never orders it.
		tx := faultrecovery.Endorse(s.II, 1)
		s.II.KillFSCNode("bob")
		s.II.StartFSCNode("bob")

		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		_, err := s.II.Client("bob").CallViewWithContext(ctx, "finality", common.JSONMarshall(&cviews.Finality{TxID: tx.TxID}))
		Expect(err).To(MatchError(ContainSubstring(fdriver.ErrFinalityTimeout.Error())))
		Expect(err).To(MatchError(ContainSubstring("last known status [busy]")))

		// The stuck tx blocks nothing.
		Expect(faultrecovery.SubmitAndWaitFinal(s.II, 1)).To(HaveLen(1))
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

// Its own Describe, so the fault proxies and the keepalive apply to these specs only.
// Each spec heals what it partitions, so the specs share one network, see
// faultrecovery.ShareNetwork.
var _ = Describe("Fault Recovery with Fabric during a network partition", Ordered, ContinueOnFailure, Label(fsc.WebSocket, partition), func() {
	var proxies *fabricfaultrecovery.FaultProxies
	s := integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		ii, err := newInfrastructure()
		if err != nil {
			return nil, err
		}
		if proxies != nil {
			proxies.Close()
		}
		proxies = fabricfaultrecovery.SetFaultProxies(ii)
		return ii, nil
	})
	faultrecovery.ShareNetwork(s)
	AfterAll(func() {
		if proxies != nil {
			proxies.Close()
		}
	})

	It("detects a silent partition of the delivery peer", func() {
		s.II.EventuallyConnectionState("alice", delivery, connstate.Connected, time.Minute)

		proxies.Peer.Partition()
		DeferCleanup(proxies.Peer.Heal)
		// Only the keepalive notices: the peer pushes nothing on a quiet channel.
		s.II.EventuallyConnectionState("alice", delivery, connstate.Reconnecting,
			fabricfaultrecovery.KeepaliveTime+fabricfaultrecovery.KeepaliveTimeout+10*time.Second)

		proxies.Peer.Heal()
		s.II.EventuallyConnectionState("alice", delivery, connstate.Connected, 2*time.Minute)
		Expect(faultrecovery.SubmitAndWaitFinal(s.II, 1)).To(HaveLen(1))
	})

	// Heal delivers the bytes the proxy held, so bob's queries blocked in the partition
	// complete on their own connections: the spec covers the database monitor and commits
	// resuming, not the committer's retry after a lost connection, which the Postgres
	// restart specs cover.
	It("resumes commits after a Postgres partition", func() {
		// Endorsing writes to bob's database, so the tx is endorsed before the partition
		// and ordered during it.
		tx := faultrecovery.Endorse(s.II, 1)

		proxies.Postgres.Partition()
		DeferCleanup(proxies.Postgres.Heal)
		s.II.EventuallyConnectionState("bob", database, connstate.Reconnecting, time.Minute)
		faultrecovery.Order(s.II, tx)

		proxies.Postgres.Heal()
		s.II.EventuallyConnectionState("bob", database, connstate.Connected, time.Minute)
		faultrecovery.WaitFinal(s.II, tx.TxID)
		Expect(faultrecovery.SubmitAndWaitFinal(s.II, 1)).To(HaveLen(1))
	})

	// The websocket between alice and bob neither pings nor, on alice's side, has a read
	// deadline, so FSC does not detect the cut: the view ends on EchoView's own receive
	// timeout. The spec checks that the cut reaches alice's sessions and that they work
	// again after Heal; detecting the cut is #2407.
	It("times out a view while alice is cut off from bob", func() {
		// Opens alice's connection to bob, so the echo below sends on it.
		Expect(faultrecovery.Echo(s.II, false)).To(Succeed())

		proxies.Bob.Partition()
		DeferCleanup(proxies.Bob.Heal)
		// The echo succeeds whenever bob gets the message.
		err := faultrecovery.Echo(s.II, false)
		Expect(err).To(MatchError(view.ErrViewExecutionFailed))
		Expect(err).To(MatchError(ContainSubstring("time out reached")))

		proxies.Bob.Heal()
		Eventually(func() error { return faultrecovery.Echo(s.II, false) }, faultrecovery.RecoveryTimeout, time.Second).Should(Succeed())
	})
})
