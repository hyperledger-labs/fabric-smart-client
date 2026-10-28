/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package faultrecovery

import (
	"net/url"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
)

// RecoveryTimeout bounds how long a restarted node may take to settle a transaction.
const RecoveryTimeout = 2 * time.Minute

// startupOutage is how long a node's database stays down after the node starts. It must
// stay below the time postgres.Open waits for the database at startup
// (postgres.DefaultStartupConnectTimeout, 60 s), or the node gives up and exits.
const startupOutage = 10 * time.Second

// sessionCloseTimeout bounds how long a view waiting on a session takes to fail after its
// counterparty dies: the comm package closes a session 5 s after its last stream died
// (comm's sessionStreamGrace), plus 10 s for the survivor to see the dead stream and for
// the error to reach the client.
const sessionCloseTimeout = 15 * time.Second

// viewContexts is the gauge of the view contexts a node holds, a responder's included.
const viewContexts = "fsc_view_services_view_contexts"

// ShareNetwork makes the specs of the enclosing Ordered, ContinueOnFailure container share
// one network, set up before the first spec and torn down after the last. Each spec in
// that container must leave the network healthy: every node it stops or kills is running
// again, every database is up, and the node configuration is unchanged. A spec that fails
// may leave the network broken, so the network is rebuilt before the next spec runs; if
// the rebuild fails, that spec fails before its body runs and the next one tries again.
// A spec that changes the network for good may share it only as the last spec of the
// container. When the suite timeout expires, ginkgo skips the remaining specs without
// running AfterAll, so the network is left running.
func ShareNetwork(s *integration.TestSuite) {
	var broken bool
	ginkgo.BeforeAll(s.Setup)
	ginkgo.AfterAll(s.TearDown)
	ginkgo.BeforeEach(func() {
		if broken {
			s.TearDown()
			s.Setup()
		}
	})
	ginkgo.AfterEach(func() { broken = ginkgo.CurrentSpecReport().Failed() })
}

// CrashSpecs registers the node crash and catch-up specs for the suite s. The recovery
// specs check bob through QueryView: on Fabric that reads bob's own vault, so the specs
// prove that bob recovers its local state; on Fabric-x, where bob has no local ledger
// state, they prove only that bob restarts and serves views again. Each spec leaves the
// network healthy, so the specs can run under ShareNetwork.
func CrashSpecs(s *integration.TestSuite) {
	ginkgo.It("fails fast when the responder dies", func() {
		echo := blockedEcho(s, s.II.Client("alice"))
		s.II.KillFSCNode("bob")
		// The echo view waits a minute for the reply, so only the closed session ends it
		// this early.
		var err error
		gomega.Eventually(echo, sessionCloseTimeout).Should(gomega.Receive(&err))
		gomega.Expect(err).To(gomega.MatchError(view.ErrViewExecutionFailed))
		gomega.Expect(err).To(gomega.MatchError(gomega.ContainSubstring("session receive channel is closed")))

		s.II.StartFSCNode("bob")
		gomega.Expect(Echo(s.II, false)).To(gomega.Succeed())
	})

	ginkgo.It("recovers a node killed mid-transaction", func() {
		// Bob has endorsed and stored the tx; alice has not ordered it yet.
		tx := Endorse(s.II, 42)
		s.II.KillFSCNode("bob")
		txID := Order(s.II, tx)
		s.II.StartFSCNode("bob")

		for _, node := range []string{"alice", "bob"} {
			gomega.Eventually(func(g gomega.Gomega) {
				res, err := Query(s.II, node, txID)
				g.Expect(err).NotTo(gomega.HaveOccurred())
				g.Expect(res.Status).To(gomega.Equal(fdriver.Valid), "status on [%s]", node)
				g.Expect(res.Value).To(gomega.Equal(42))
			}, RecoveryTimeout, time.Second).Should(gomega.Succeed(), "tx [%s] not settled on [%s]", txID, node)
		}
	})

	ginkgo.It("catches up blocks committed while down", func() {
		// Bob endorses every tx, then misses the blocks that commit them.
		txs := make([]*views.Endorsed, 20)
		for i := range txs {
			txs[i] = Endorse(s.II, i)
		}
		s.II.StopFSCNode("bob")
		txIDs := make([]string, len(txs))
		for i, tx := range txs {
			txIDs[i] = Order(s.II, tx)
		}
		s.II.StartFSCNode("bob")

		// Each tx gets its own budget, so a failure names the tx bob has not settled.
		for i, txID := range txIDs {
			gomega.Eventually(func(g gomega.Gomega) {
				res, err := Query(s.II, "bob", txID)
				g.Expect(err).NotTo(gomega.HaveOccurred())
				g.Expect(res).To(gomega.Equal(&views.QueryResult{Status: fdriver.Valid, Value: i}))
			}, RecoveryTimeout, time.Second).Should(gomega.Succeed(), "tx %d [%s] not caught up on bob", i, txID)
		}
	})
}

// ClientSpecs registers, once for alice's gRPC view client and once for her web client,
// the specs that pin how a view client behaves across a restart of its node: the client
// value nwo built before the restart keeps working, and a call in flight when the node
// dies fails without being retried. Each spec leaves the network healthy, so the specs
// can run under ShareNetwork.
func ClientSpecs(s *integration.TestSuite) {
	// A slice, not a map: ginkgo needs the same spec order on every run.
	clients := []struct {
		name string
		get  func() api.ViewClient
		// failed matches the error of a call in flight when the node dies.
		failed types.GomegaMatcher
	}{
		{
			"grpc", func() api.ViewClient { return s.II.Client("alice") },
			gomega.WithTransform(status.Code, gomega.Equal(codes.Unavailable)),
		},
		{
			"web", func() api.ViewClient { return s.II.WebClient("alice") },
			gomega.WithTransform(func(err error) bool { return errors.HasType(err, &url.Error{}) }, gomega.BeTrue()),
		},
	}
	for _, cl := range clients {
		ginkgo.It("reuses the "+cl.name+" view client after a restart", func() {
			c := cl.get()
			gomega.Expect(EchoWith(s.II, c, false)).To(gomega.Succeed())
			s.II.StopFSCNode("alice")
			s.II.StartFSCNode("alice")
			gomega.Eventually(func() error { return EchoWith(s.II, c, false) }, RecoveryTimeout, time.Second).Should(gomega.Succeed())
		})

		ginkgo.It("fails an in-flight "+cl.name+" call when the node dies", func() {
			c := cl.get()
			echo := blockedEcho(s, c)
			// blockedEcho saw exactly this call's responder.
			gomega.Expect(MetricSum(s.II, "bob", viewContexts)).To(gomega.Equal(1.0))
			s.II.KillFSCNode("alice")
			// The call's own deadline is viewTimeout, so a client that retried it would
			// still be blocked after 10 s.
			var err error
			gomega.Eventually(echo, 10*time.Second).Should(gomega.Receive(&err))
			gomega.Expect(err).To(cl.failed)

			s.II.StartFSCNode("alice")
			gomega.Eventually(func() error { return EchoWith(s.II, c, false) }, RecoveryTimeout, time.Second).Should(gomega.Succeed())
		})
	}
}

// blockedEcho starts a blocking echo through c, alice's view client, and returns once
// bob's responder blocks in it. The returned channel receives the echo's error.
func blockedEcho(s *integration.TestSuite, c api.ViewClient) <-chan error {
	// On a shared network the previous spec's responder may still hold its context for
	// a moment after its view ends, so the baseline waits for it.
	gomega.EventuallyWithOffset(1, func() float64 {
		return MetricSum(s.II, "bob", viewContexts)
	}, pollTimeout, 100*time.Millisecond).Should(gomega.BeZero())
	echo := make(chan error, 1)
	go func() {
		defer ginkgo.GinkgoRecover()
		echo <- EchoWith(s.II, c, true)
	}()
	// Bob creates a responder context when alice's message arrives and holds it while
	// the responder blocks, so a kill lands mid-view. An echo that ends before then
	// stops the wait with its own error.
	gomega.EventuallyWithOffset(1, func(g gomega.Gomega) float64 {
		select {
		case err := <-echo:
			gomega.StopTrying("echo returned before bob's responder blocked").Wrap(err).Now()
		default:
		}
		families, err := s.II.WebClient("bob").Metrics()
		g.Expect(err).NotTo(gomega.HaveOccurred())
		return sumFamily(families, viewContexts)
	}, pollTimeout, 100*time.Millisecond).Should(gomega.BeNumerically(">", 0))
	return echo
}

// DependencySpecs registers the specs that take down a dependency both suites share: bob's
// Postgres database, named DBName(dbPrefix, "bob"). Each spec brings the database back, so
// the specs can run under ShareNetwork.
func DependencySpecs(s *integration.TestSuite, dbPrefix string) {
	ginkgo.It("starts before Postgres is up", func() {
		db := DBName(dbPrefix, "bob")
		s.II.StopFSCNode("bob")
		s.II.StopPostgres(db)

		// StartFSCNode returns only once bob has started, which it cannot do without its
		// database, so the database comes back from another goroutine. The sleep is the
		// outage itself: no signal reports that bob is waiting for its database.
		started := make(chan struct{})
		go func() {
			defer ginkgo.GinkgoRecover()
			defer close(started)
			time.Sleep(startupOutage)
			s.II.StartPostgres(db)
		}()
		// Waits even if StartFSCNode fails, so the goroutine does not outlive a failing spec.
		defer func() { <-started }()
		s.II.StartFSCNode("bob")

		gomega.Eventually(s.II.WebClient("bob").Ready, RecoveryTimeout, time.Second).Should(gomega.Succeed())
		gomega.Expect(SubmitAndWaitFinal(s.II, 1)).To(gomega.HaveLen(1))
	})
}
