/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package faultrecovery

import (
	"context"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
)

// RecoveryTimeout bounds how long a restarted node may take to settle a transaction.
const RecoveryTimeout = 2 * time.Minute

// startupOutage is how long a node's database stays down after the node starts. It must
// stay below the time postgres.Open waits for the database at startup
// (postgres.DefaultStartupConnectTimeout, 60 s), or the node gives up and exits.
const startupOutage = 10 * time.Second

// SessionCloseTimeout bounds how long a view waiting on a session takes to fail after its
// counterparty dies: the comm package closes a session 5 s after its last stream died
// (comm's sessionStreamGrace), plus 10 s for the survivor to see the dead stream and for
// the error to reach the client.
const SessionCloseTimeout = 15 * time.Second

const (
	// ViewContexts is the gauge of the view contexts a node holds, a responder's included.
	ViewContexts = "fsc_view_services_view_contexts"
	// Sessions is the gauge of the sessions a node holds.
	Sessions = "fsc_view_services_comm_sessions"
)

// ShareNetwork makes the specs of the enclosing Ordered, ContinueOnFailure container share
// one network, set up before the first spec and torn down after the last. Each spec in
// that container must leave the network healthy: every node it stops or kills is running
// again, every database is up, and the node configuration is unchanged. A spec that fails
// may leave the network broken, so the network is rebuilt before the next spec runs; if
// the rebuild fails, that spec fails before its body runs and the next one tries again.
// A spec that changes the network for good may share it only as the last spec of the
// container. The network is torn down by a DeferCleanup registered in BeforeAll, which
// ginkgo runs after the DeferCleanup of the last spec, so a spec may restart a node it
// killed in a DeferCleanup. When the suite timeout expires, ginkgo skips the remaining
// specs without tearing down, so the network is left running.
//
// After each spec that passed, every FSC node and replica must hold no more sessions and
// view contexts than before it, see expectNoLeaks; a spec that leaks fails. The check runs
// in a DeferCleanup registered before the spec body, so it runs after the spec's own
// cleanups, once the nodes they restart are back.
func ShareNetwork(s *integration.TestSuite) {
	var broken bool
	ginkgo.BeforeAll(func() {
		s.Setup()
		ginkgo.DeferCleanup(s.TearDown)
	})
	ginkgo.BeforeEach(func() {
		if broken {
			s.TearDown()
			s.Setup()
		}
		// Cleanups run last-registered first: the leak check, then the bookkeeping.
		ginkgo.DeferCleanup(func() { broken = ginkgo.CurrentSpecReport().Failed() })
		before := leakGauges(s.II)
		ginkgo.DeferCleanup(func() {
			// A failed spec may leave nodes down; the network is rebuilt anyway.
			if !ginkgo.CurrentSpecReport().Failed() {
				expectNoLeaks(s.II, before)
			}
		})
	})
}

// leakGauges returns the Sessions and ViewContexts gauges of every FSC node and replica of
// ii, keyed by replica ID and gauge, once they have been unchanged for 2 s: a responder of
// the previous spec may still hold its context for a moment. The wait is shorter than
// comm's sessionStreamGrace (5 s), so a session whose stream died may still be counted.
// That only raises the baseline: the check may miss a leak but never reports a false one.
func leakGauges(ii *integration.Infrastructure) map[string]float64 {
	var gauges map[string]float64
	gomega.EventuallyWithOffset(1, func(g gomega.Gomega) {
		gauges = readLeakGauges(g, ii)
		g.Consistently(func(g gomega.Gomega) map[string]float64 {
			return readLeakGauges(g, ii)
		}, 2*time.Second, 500*time.Millisecond).Should(gomega.Equal(gauges))
	}, time.Minute, time.Second).Should(gomega.Succeed())
	return gauges
}

func readLeakGauges(g gomega.Gomega, ii *integration.Infrastructure) map[string]float64 {
	gauges := map[string]float64{}
	for _, r := range ii.FscPlatform.Peers {
		families, err := ii.WebClient(r.ID()).Metrics()
		g.Expect(err).NotTo(gomega.HaveOccurred(), "metrics of [%s]", r.ID())
		for _, name := range []string{Sessions, ViewContexts} {
			gauges[r.ID()+" "+name] = sumFamily(families, name)
		}
	}
	return gauges
}

// expectNoLeaks waits up to SessionCloseTimeout until, on every FSC node and replica of ii,
// the Sessions and ViewContexts gauges are at most their value in before.
func expectNoLeaks(ii *integration.Infrastructure, before map[string]float64) {
	gomega.EventuallyWithOffset(1, func(g gomega.Gomega) {
		for k, v := range readLeakGauges(g, ii) {
			g.Expect(v).To(gomega.BeNumerically("<=", before[k]), "[%s] was %v before the spec", k, before[k])
		}
	}, SessionCloseTimeout, time.Second).Should(gomega.Succeed(), "sessions or view contexts leaked")
}

// CrashSpecs registers the node crash and catch-up specs for the suite s, with responder,
// an FSC node name, as the node that crashes. Each spec name ends in "(<responder>)" and
// carries the label crash, so a suite can run them for several responders and in a target
// of their own. The recovery specs check the responder through QueryView: on Fabric that
// reads its own vault, so the specs prove that it recovers its local state; on Fabric-x,
// where it has no local ledger state, they prove only that it restarts and serves views
// again. Each spec leaves the network healthy, so the specs can run under ShareNetwork.
func CrashSpecs(s *integration.TestSuite, responder string) {
	label := ginkgo.Label("crash")
	name := func(spec string) string { return spec + " (" + responder + ")" }
	endorse := func(value int) *views.Endorsed { return EndorseFrom(s.II, value, responder) }

	ginkgo.It(name("fails fast when the responder dies"), label, func() {
		echo := BlockedEcho(s.II, s.II.Client("alice"), responder, responder)
		s.II.KillFSCNode(responder)
		// The echo view waits a minute for the reply, so only the closed session ends it
		// this early.
		var err error
		gomega.Eventually(echo, SessionCloseTimeout).Should(gomega.Receive(&err))
		gomega.Expect(err).To(gomega.MatchError(view.ErrViewExecutionFailed))
		gomega.Expect(err).To(gomega.MatchError(gomega.ContainSubstring("session receive channel is closed")))

		s.II.StartFSCNode(responder)
		gomega.Expect(EchoTo(s.II, s.II.Client("alice"), responder, false)).To(gomega.Succeed())
	})

	ginkgo.It(name("recovers a node killed mid-transaction"), label, func() {
		// The responder has endorsed and stored the tx; alice has not ordered it yet.
		tx := endorse(42)
		s.II.KillFSCNode(responder)
		txID := Order(s.II, tx)
		s.II.StartFSCNode(responder)

		for _, node := range []string{"alice", responder} {
			gomega.Eventually(func(g gomega.Gomega) {
				res, err := Query(s.II, node, txID)
				g.Expect(err).NotTo(gomega.HaveOccurred())
				g.Expect(res.Status).To(gomega.Equal(fdriver.Valid), "status on [%s]", node)
				g.Expect(res.Value).To(gomega.Equal(42))
			}, RecoveryTimeout, time.Second).Should(gomega.Succeed(), "tx [%s] not settled on [%s]", txID, node)
		}
	})

	// missBlocks has the responder endorse n txs, tx i with value i, and stops it while alice
	// orders them, so it misses the blocks that commit them. It restarts the responder and
	// returns the IDs of the txs.
	missBlocks := func(n int) []string {
		txs := make([]*views.Endorsed, n)
		for i := range txs {
			txs[i] = endorse(i)
		}
		s.II.StopFSCNode(responder)
		txIDs := make([]string, n)
		for i, tx := range txs {
			txIDs[i] = Order(s.II, tx)
		}
		s.II.StartFSCNode(responder)
		return txIDs
	}
	// expectCaughtUp gives each tx missBlocks returned its own budget, so a failure names
	// the tx the responder has not settled.
	expectCaughtUp := func(txIDs []string) {
		for i, txID := range txIDs {
			ExpectSettled(s.II, responder, txID, i)
		}
	}

	ginkgo.It(name("catches up blocks committed while down"), label, func() {
		expectCaughtUp(missBlocks(20))
	})

	ginkgo.It(name("settles a tx whose initiator died after broadcasting"), label, func() {
		// Alice broadcasts the tx and dies before she sees it final, so she learns its
		// status only after the restart, from the block she catches up on. If the block is
		// delivered before the kill lands, the spec checks only that alice restarts with it.
		txID := Broadcast(s.II, endorse(7))
		s.II.KillFSCNode("alice")
		s.II.StartFSCNode("alice")

		for _, node := range []string{"alice", responder} {
			ExpectSettled(s.II, node, txID, 7)
		}
	})

	ginkgo.It(name("catches up after a crash during catch-up"), label, func() {
		// The responder dies again as soon as it has caught up on the first tx it missed:
		// the second restart must still catch up on all of them, with no gap.
		txIDs := missBlocks(20)
		gomega.Eventually(func(g gomega.Gomega) {
			res, err := Query(s.II, responder, txIDs[0])
			g.Expect(err).NotTo(gomega.HaveOccurred())
			g.Expect(res.Status).To(gomega.Equal(fdriver.Valid))
		}, RecoveryTimeout, 100*time.Millisecond).Should(gomega.Succeed(), "[%s] never caught up on the first tx", responder)
		// The catch-up may finish within one poll, and then the kill lands after it. The log
		// says so, as asserting it would make the spec flaky.
		if res, err := Query(s.II, responder, txIDs[len(txIDs)-1]); err == nil && res.Status == fdriver.Valid {
			ginkgo.GinkgoWriter.Printf("[%s] had caught up on every tx before the kill\n", responder)
		}
		s.II.KillFSCNode(responder)
		s.II.StartFSCNode(responder)

		expectCaughtUp(txIDs)
	})

	ginkgo.It(name("loses no tx when the responder dies under load"), label, func() {
		const workers = 4

		// The workers commit 10 txs, then the responder dies with up to one call of each
		// in flight. Each may lose that call, and no other: the workers stop at the kill
		// and start again only once the responder serves an endorsement, so no call
		// reaches it while it is down or still starting.
		before := runLoad(s.II, workers, 10, func() { s.II.KillFSCNode(responder) }, responder)
		s.II.StartFSCNode(responder)
		var probe string
		gomega.Eventually(func() (err error) {
			probe, err = TrySubmitTo(s.II, 0, responder)
			return err
		}, RecoveryTimeout, time.Second).Should(gomega.Succeed(), "[%s] never endorsed again after the restart", responder)

		// The workers commit 10 more against the restarted responder.
		after := runLoad(s.II, workers, 10, nil, responder)
		gomega.Expect(before.failed).To(gomega.BeNumerically("<=", workers), "calls lost at the kill")
		gomega.Expect(after.failed).To(gomega.BeZero(), "calls lost after the restart")

		txIDs := append(append(before.txIDs, probe), after.txIDs...)
		for _, txID := range txIDs {
			for _, node := range []string{"alice", responder} {
				gomega.Eventually(func(g gomega.Gomega) {
					res, err := Query(s.II, node, txID)
					g.Expect(err).NotTo(gomega.HaveOccurred())
					g.Expect(res.Status).To(gomega.Equal(fdriver.Valid))
				}, RecoveryTimeout, time.Second).Should(gomega.Succeed(), "tx [%s] a client was told succeeded is not final on [%s]", txID, node)
			}
		}
	})
}

// load is the outcome of runLoad: the IDs of the txs a client was told succeeded, those
// whose order view returned, and the number of calls that returned an error.
type load struct {
	txIDs  []string
	failed int
}

// runLoad runs workers goroutines, each looping TrySubmitTo with responders, until n
// submissions have succeeded. It then stops the workers and calls interrupt, if any, so
// interrupt lands with each worker's call, if it is in one, in flight, and no worker
// starts another after it. It returns once every worker has finished.
func runLoad(ii *integration.Infrastructure, workers, n int, interrupt func(), responders ...string) load {
	var (
		mu        sync.Mutex
		wg        sync.WaitGroup
		res       load
		succeeded atomic.Int64
	)
	stop := make(chan struct{})
	for w := range workers {
		wg.Go(func() {
			for i := w; ; i += workers {
				select {
				case <-stop:
					return
				default:
				}
				txID, err := TrySubmitTo(ii, i, responders...)
				mu.Lock()
				if err != nil {
					res.failed++
				} else {
					res.txIDs = append(res.txIDs, txID)
				}
				mu.Unlock()
				if err == nil {
					succeeded.Add(1)
				}
			}
		})
	}
	closeStop := sync.OnceFunc(func() { close(stop) })
	// Stops the workers even if the wait fails, so none outlives the spec.
	defer closeStop()
	gomega.EventuallyWithOffset(1, succeeded.Load, RecoveryTimeout, 100*time.Millisecond).Should(gomega.BeNumerically(">=", n))
	closeStop()
	if interrupt != nil {
		interrupt()
	}
	wg.Wait()
	return res
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
		// cancelled matches the error of a call whose context the caller cancelled.
		cancelled types.GomegaMatcher
	}{
		{
			"grpc", func() api.ViewClient { return s.II.Client("alice") },
			gomega.WithTransform(status.Code, gomega.Equal(codes.Unavailable)),
			gomega.WithTransform(status.Code, gomega.Equal(codes.Canceled)),
		},
		{
			"web", func() api.ViewClient { return s.II.WebClient("alice") },
			gomega.WithTransform(func(err error) bool { return errors.HasType(err, &url.Error{}) }, gomega.BeTrue()),
			gomega.MatchError(context.Canceled),
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
			echo := BlockedEcho(s.II, c, "bob", "bob")
			// BlockedEcho saw exactly this call's responder.
			gomega.Expect(MetricSum(s.II, "bob", ViewContexts)).To(gomega.Equal(1.0))
			s.II.KillFSCNode("alice")
			// The call's own deadline is viewTimeout, so a client that retried it would
			// still be blocked after 10 s.
			var err error
			gomega.Eventually(echo, 10*time.Second).Should(gomega.Receive(&err))
			gomega.Expect(err).To(cl.failed)

			s.II.StartFSCNode("alice")
			gomega.Eventually(func() error { return EchoWith(s.II, c, false) }, RecoveryTimeout, time.Second).Should(gomega.Succeed())
		})

		ginkgo.It("frees the responder when the "+cl.name+" caller cancels", func() {
			c := cl.get()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			echo := BlockedEchoWithContext(ctx, s.II, c, "bob", "bob")
			cancel()

			var err error
			gomega.Eventually(echo, SessionCloseTimeout).Should(gomega.Receive(&err))
			gomega.Expect(err).To(cl.cancelled)
			// The leak check after the spec proves bob freed the responder's view context;
			// this states it where the spec is about it.
			gomega.Eventually(func() float64 {
				return MetricSum(s.II, "bob", ViewContexts)
			}, SessionCloseTimeout, 100*time.Millisecond).Should(gomega.BeZero(), "bob's responder still holds its view context")
		})
	}
}

// ViewSpecs registers the specs that run a view with more than one responder, for the suite
// s. They are about the view, not about which node crashes, so a suite calls ViewSpecs once,
// unlike CrashSpecs. Each spec leaves the network healthy, so the specs can run under
// ShareNetwork.
func ViewSpecs(s *integration.TestSuite) {
	ginkgo.It("fails a three-party view when one party is down", func() {
		// Alice collects from bob, then from charlie, who is dead: the view must fail on
		// charlie's session closing rather than hang, and bob must release his half.
		s.II.KillFSCNode("charlie")
		up := false
		// A failure while charlie is down would leave him down for the next spec.
		ginkgo.DeferCleanup(func() {
			if !up {
				s.II.StartFSCNode("charlie")
			}
		})
		failed := make(chan error, 1)
		go func() {
			defer ginkgo.GinkgoRecover()
			_, err := TryEndorseFrom(s.II, 1, "bob", "charlie")
			failed <- err
		}()
		var err error
		gomega.Eventually(failed, SessionCloseTimeout).Should(gomega.Receive(&err))
		gomega.Expect(err).To(gomega.HaveOccurred(), "the endorse view succeeded with charlie dead")
		gomega.Eventually(func() float64 {
			return MetricSum(s.II, "bob", ViewContexts)
		}, SessionCloseTimeout, 100*time.Millisecond).Should(gomega.BeZero(), "bob's responder still holds its view context")

		s.II.StartFSCNode("charlie")
		up = true
		txID := Order(s.II, EndorseFrom(s.II, 2, "bob", "charlie"))
		for _, node := range []string{"alice", "bob", "charlie"} {
			WaitFinalOn(s.II, node, txID)
		}
	})
}

// FailoverSpecs registers the specs that kill the replica of bob holding alice's connection
// through bob's load balancer, for a suite s on ReplicatedFSCTopology. conn is the
// connection a restarted replica of bob must report Connected again before the next spec
// runs. Each spec leaves every replica running, so the specs can run under ShareNetwork.
func FailoverSpecs(s *integration.TestSuite, conn fsc.Connection) {
	ginkgo.It("serves new sessions after the replica holding them dies", func() {
		// The first tx opens alice's connection, which ConnectedReplica looks for.
		gomega.Expect(SubmitAndWaitFinal(s.II, 1)).To(gomega.HaveLen(1))
		held, others := ConnectedReplica(s.II, "bob")
		gomega.Expect(others).To(gomega.HaveLen(1))
		survivor := others[0]
		s.II.KillFSCNode(held.UniqueName)

		// The kill closes alice's connection, so her next session dials the load balancer
		// again and reaches the survivor.
		tx := EndorseEventually(s.II, 1)
		ExpectSettled(s.II, survivor.ID(), Order(s.II, tx), 1)
		now, _ := ConnectedReplica(s.II, "bob")
		gomega.Expect(now.UniqueName).To(gomega.Equal(survivor.UniqueName))

		s.II.StartFSCNode(held.UniqueName)
		s.II.EventuallyConnectionState(held.ID(), conn, connstate.Connected, time.Minute)
		txIDs := SubmitAndWaitFinal(s.II, 10)
		last := len(txIDs) - 1
		for _, r := range []string{held.ID(), survivor.ID()} {
			ExpectSettled(s.II, r, txIDs[last], last)
		}
	})

	ginkgo.It("continues on the other replica when the load balancer drops the connection", func() {
		// An echo opens alice's connection, which ConnectedReplica looks for.
		gomega.Expect(Echo(s.II, false)).To(gomega.Succeed())
		held, others := ConnectedReplica(s.II, "bob")
		gomega.Expect(others).To(gomega.HaveLen(1))

		// Both replicas stay up: the balancer closes the connection on its own, as it does on
		// an idle timeout or a restart, and both ends read the end of the stream at once.
		echo := BlockedEcho(s.II, s.II.Client("alice"), "bob", held.ID())
		s.II.FscPlatform.LoadBalancer("bob").CloseConnections()
		// The echo view waits a minute for the reply, so only the closed session ends it
		// this early.
		var err error
		gomega.Eventually(echo, SessionCloseTimeout).Should(gomega.Receive(&err))
		gomega.Expect(err).To(gomega.MatchError(view.ErrViewExecutionFailed))
		// The replica that lost the connection is still up and closes its half on its own:
		// the responder's view context is released, and ShareNetwork's leak check holds the
		// Sessions and ViewContexts gauges of every replica to their baseline after the spec.
		gomega.Eventually(func() float64 {
			return MetricSum(s.II, held.ID(), ViewContexts)
		}, SessionCloseTimeout, 100*time.Millisecond).Should(gomega.BeZero())

		// Alice dials the balancer again, and the round robin may hand her to either replica.
		gomega.Expect(SubmitAndWaitFinal(s.II, 1)).To(gomega.HaveLen(1))
		now, _ := ConnectedReplica(s.II, "bob")
		ginkgo.GinkgoWriter.Printf("alice's connection moved from [%s] to [%s]\n", held.UniqueName, now.UniqueName)
	})

	ginkgo.It("fails an in-flight view cleanly when its replica dies", func() {
		// An echo opens alice's connection, which ConnectedReplica looks for.
		gomega.Expect(Echo(s.II, false)).To(gomega.Succeed())
		held, others := ConnectedReplica(s.II, "bob")
		gomega.Expect(others).To(gomega.HaveLen(1))
		survivor := others[0]

		echo := BlockedEcho(s.II, s.II.Client("alice"), "bob", held.ID())
		s.II.KillFSCNode(held.UniqueName)
		// The echo view waits a minute for the reply, so only the closed session ends it
		// this early, and a retried view would still be blocked.
		var err error
		gomega.Eventually(echo, SessionCloseTimeout).Should(gomega.Receive(&err))
		gomega.Expect(err).To(gomega.MatchError(view.ErrViewExecutionFailed))
		gomega.Expect(err).To(gomega.MatchError(gomega.ContainSubstring("session receive channel is closed")))
		// No message of the failed view reaches the survivor: it would open a responder
		// context for it.
		gomega.Consistently(func() float64 {
			return MetricSum(s.II, survivor.ID(), ViewContexts)
		}, 2*time.Second, 100*time.Millisecond).Should(gomega.BeZero())

		gomega.Expect(Echo(s.II, false)).To(gomega.Succeed())
		now, _ := ConnectedReplica(s.II, "bob")
		gomega.Expect(now.UniqueName).To(gomega.Equal(survivor.UniqueName))
		s.II.StartFSCNode(held.UniqueName)
		s.II.EventuallyConnectionState(held.ID(), conn, connstate.Connected, time.Minute)
	})
}

// RollingRestartSpecs registers the spec that stops each replica of bob gracefully and
// restarts it in turn while alice submits, for a suite s on ReplicatedFSCTopology. conn is
// the connection a restarted replica must report Connected again before the next restart.
// The spec leaves every replica running, so it can run under ShareNetwork.
//
// The stopping replica drains: alice's sessions in progress finish, and a new one gets
// "draining". The test load balancer does not probe /readyz, so alice's connection stays on
// the draining replica until that reply; alice then drops it, and her next dial goes
// round-robin to the other replica. So at most one submission fails per restart, and a
// failed submission must not commit: alice got an error, so a committed tx would be a
// success she never saw.
func RollingRestartSpecs(s *integration.TestSuite, conn fsc.Connection) {
	ginkgo.It("keeps committing through a rolling restart of one replica", func() {
		// The first tx opens alice's connection, which ConnectedReplica looks for.
		gomega.Expect(SubmitAndWaitFinal(s.II, 1)).To(gomega.HaveLen(1))
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan []Submission, 1)
		go func() {
			defer ginkgo.GinkgoRecover()
			done <- SubmitUntil(ctx, s.II)
		}()

		// Only the replica holding alice's connection drains a session; restarting the idle
		// one tests nothing. Restarting every replica in turn restarts the holder at least once.
		drained := 0
		for _, r := range replicaIDs(s.II, "bob") {
			time.Sleep(2 * time.Second) // submissions run
			if held, _ := ConnectedReplica(s.II, "bob"); held.ID() == r {
				drained++
			}
			s.II.StopFSCNode(r)
			s.II.StartFSCNode(r)
			gomega.Eventually(s.II.WebClient(r).Ready, time.Minute, time.Second).Should(gomega.Succeed())
			s.II.EventuallyConnectionState(r, conn, connstate.Connected, time.Minute)
		}
		gomega.Expect(drained).To(gomega.BeNumerically(">=", 1), "no restarted replica held alice's connection")
		time.Sleep(2 * time.Second)
		cancel()
		var subs []Submission
		gomega.Eventually(done, viewTimeout).Should(gomega.Receive(&subs), "submissions did not stop")

		failed := 0
		var committed []Submission
		for _, sub := range subs {
			if sub.Err != nil {
				ginkgo.GinkgoWriter.Printf("submission %d [%s] failed: %v\n", sub.Value, sub.TxID, sub.Err)
				failed++
				if sub.TxID != "" {
					gomega.Consistently(func(g gomega.Gomega) {
						res, err := Query(s.II, "bob", sub.TxID)
						g.Expect(err).NotTo(gomega.HaveOccurred())
						g.Expect(res.Status).NotTo(gomega.Equal(fdriver.Valid))
					}, 5*time.Second, time.Second).Should(gomega.Succeed(), "failed tx [%s] committed", sub.TxID)
				}
				continue
			}
			committed = append(committed, sub)
		}
		gomega.Expect(failed).To(gomega.BeNumerically("<=", drained), "at most one failed submission per drain")
		gomega.Expect(committed).NotTo(gomega.BeEmpty())
		// The order view returns once the tx is final on alice, so only bob is checked.
		for _, sub := range committed {
			for _, r := range replicaIDs(s.II, "bob") {
				ExpectSettled(s.II, r, sub.TxID, sub.Value)
			}
		}
	})
}

// replicaIDs returns the replica IDs of node, such as fsc.bob.0, in topology order.
func replicaIDs(ii *integration.Infrastructure, node string) []string {
	var ids []string
	for _, r := range ii.FscPlatform.Peers {
		if r.Name == node {
			ids = append(ids, r.ID())
		}
	}
	return ids
}

// BlockedEcho starts a blocking echo to node, an FSC node name, through c, alice's view
// client, and returns once node's responder blocks in it on replica, node's name or one of
// its replica IDs. The returned channel receives the echo's error.
func BlockedEcho(ii *integration.Infrastructure, c api.ViewClient, node, replica string) <-chan error {
	return startBlockedEcho(context.Background(), ii, c, node, replica)
}

// BlockedEchoWithContext is BlockedEcho whose call ends when ctx does, so a spec can cancel
// a call the responder is blocked in, or let its deadline expire.
func BlockedEchoWithContext(ctx context.Context, ii *integration.Infrastructure, c api.ViewClient, node, replica string) <-chan error {
	return startBlockedEcho(ctx, ii, c, node, replica)
}

// startBlockedEcho implements BlockedEcho and BlockedEchoWithContext. Its assertions report
// the line of their caller.
func startBlockedEcho(ctx context.Context, ii *integration.Infrastructure, c api.ViewClient, node, replica string) <-chan error {
	// On a shared network the previous spec's responder may still hold its context for
	// a moment after its view ends, so the baseline waits for it.
	gomega.EventuallyWithOffset(2, func() float64 {
		return MetricSum(ii, replica, ViewContexts)
	}, pollTimeout, 100*time.Millisecond).Should(gomega.BeZero())
	echo := make(chan error, 1)
	go func() {
		defer ginkgo.GinkgoRecover()
		echo <- callEcho(ctx, c, &views.Echo{Responder: ii.Identity(node), Block: true})
	}()
	// The responder creates a context when alice's message arrives and holds it while it
	// blocks, so a kill lands mid-view. An echo that ends before then stops the wait with
	// its own error.
	gomega.EventuallyWithOffset(2, func(g gomega.Gomega) float64 {
		select {
		case err := <-echo:
			gomega.StopTrying("echo returned before the responder of [" + replica + "] blocked").Wrap(err).Now()
		default:
		}
		families, err := ii.WebClient(replica).Metrics()
		g.Expect(err).NotTo(gomega.HaveOccurred())
		return sumFamily(families, ViewContexts)
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

// HostileSpecs registers the hostile counterparty specs for the suite s. Each spec
// leaves the network healthy, so the specs can run under ShareNetwork. The forged txID
// spec skips on Fabric-x, whose endorsers do not check that the txID is derived from
// nonce and creator.
func HostileSpecs(s *integration.TestSuite) {
	ginkgo.It("rejects an endorsement over a changed read-write set", func() {
		rejected := EndorseLiar(s.II, 1)
		gomega.Expect(rejected.TxID).NotTo(gomega.BeEmpty())
		gomega.Expect(rejected.Responder).To(gomega.Equal("bob"))
		gomega.Expect(rejected.Reason).To(gomega.ContainSubstring("received different results"))

		// Both nodes opened the transaction's read-write set, which on Fabric marks it Busy
		// in their vaults; on Fabric-x, where status reads the committer, it is Unknown.
		// Neither node may report it Valid or Invalid, since it was never ordered.
		for _, node := range []string{"alice", "bob"} {
			res, err := Query(s.II, node, rejected.TxID)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(res.Status).To(gomega.Or(gomega.Equal(fdriver.Unknown), gomega.Equal(fdriver.Busy)), "status on [%s]", node)
		}

		gomega.Expect(SubmitAndWaitFinal(s.II, 1)).To(gomega.HaveLen(1))
	})

	ginkgo.It("refuses a transaction whose txID the sender chose", func() {
		if nwofabricx.FxPlatform(s.II) != nil {
			ginkgo.Skip("Fabric-x does not derive the txID from nonce and creator, so endorsers do not check it; see docs/platform/view/security-model.md")
		}
		rejected := EndorseForgedTxID(s.II, 1)
		gomega.Expect(rejected.TxID).NotTo(gomega.BeEmpty())
		gomega.Expect(rejected.Responder).To(gomega.Equal("bob"))
		gomega.Expect(rejected.Reason).To(gomega.ContainSubstring("txID is not derived from nonce and creator"))

		// Bob refuses the transaction before it opens its read-write set, so his vault
		// holds nothing under the forged txID.
		res, err := Query(s.II, "bob", rejected.TxID)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(res.Status).To(gomega.Equal(fdriver.Unknown))

		gomega.Expect(SubmitAndWaitFinal(s.II, 1)).To(gomega.HaveLen(1))
	})

	ginkgo.It("refuses a recipient identity claimed without proof of ownership", func() {
		// Alice claims bob's own default identity, which she holds no key for.
		claimed := RecipientOwner(s.II, nil)
		gomega.Expect(claimed.Name).To(gomega.Equal("bob"))

		refused := ExchangeForgedRecipient(s.II, claimed.Identity)
		gomega.Expect(refused.Reason).To(gomega.ContainSubstring("received error from remote [recipient proof: missing]"))

		// Bob bound nothing: the claimed identity still routes to bob, not to alice.
		gomega.Expect(RecipientOwner(s.II, claimed.Identity).Name).To(gomega.Equal("bob"))

		// An exchange with proofs still binds each identity to its owner.
		exchanged := ExchangeRecipients(s.II)
		gomega.Expect(exchanged.Receiver).To(gomega.Equal(claimed.Identity))
		gomega.Expect(RecipientOwner(s.II, exchanged.Sender).Name).To(gomega.Equal("alice"))

		gomega.Expect(SubmitAndWaitFinal(s.II, 1)).To(gomega.HaveLen(1))
	})
}
