/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorsers

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/endorsers/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery"
	fviews "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery/views"
	cviews "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

const (
	// AttemptTimeout bounds alice's attempt on one endorser. A healthy endorser answers
	// well within it, and a hung one costs alice AttemptTimeout instead of the view's
	// one-minute timer.
	AttemptTimeout = 5 * time.Second
	// Cooldown is how long alice asks an endorser that failed only after the others.
	Cooldown = 10 * time.Second
	// viewTimeout bounds each view call, so a node that never answers fails the spec
	// instead of hanging it.
	viewTimeout = 2 * time.Minute
	// rejoinTimeout bounds how long a restarted endorser takes to sign again: it has to
	// recover, and alice asks it first again only once Cooldown has passed.
	rejoinTimeout = Cooldown + faultrecovery.RecoveryTimeout
)

// Specs registers the specs of an endorsers suite for s, a suite on FSCTopology under
// faultrecovery.ShareNetwork; its Describe must be Ordered. Each spec leaves every node
// running. The last one checks the transaction the spec before it sent to the endorser it
// paused, so the two run in that order.
func Specs(s *integration.TestSuite) {
	ginkgo.It("spreads endorsements over every endorser", func() {
		signed := map[string]int{}
		for i := range 30 {
			sub := submit(s.II, i)
			gomega.Expect(sub.Err).NotTo(gomega.HaveOccurred(), "tx with value %d", i)
			signed[sub.Endorser]++
		}
		for _, e := range Endorsers {
			gomega.Expect(signed).To(gomega.HaveKeyWithValue(e, gomega.BeNumerically(">", 0)), "every endorser signs some txs")
		}
	})

	ginkgo.It("fails over when an endorser is killed under load", func() {
		var killed time.Time
		subs := underLoad(s.II, func() {
			s.II.KillFSCNode(Endorsers[0])
			killed = time.Now()
		})
		for _, sub := range subs {
			gomega.Expect(sub.Err).NotTo(gomega.HaveOccurred(), "tx with value %d", sub.Value)
			if sub.Started.After(killed) {
				gomega.Expect(sub.Endorser).NotTo(gomega.Equal(Endorsers[0]), "tx with value %d started after the kill", sub.Value)
			}
		}
		s.II.StartFSCNode(Endorsers[0])
		expectSignsAgain(s.II, Endorsers[0])
	})

	ginkgo.It("moves on when an endorser drains", func() {
		// A view running on endorser1 holds its drain: here a finality wait for a tx alice
		// has endorsed but not ordered. endorser1 drains until alice orders the tx, or for at
		// most its drain timeout (fsc.shutdown.drainTimeout, 30 s by default).
		held := endorse(s.II, 300, endorsersOf(s.II)[:1])
		gomega.Expect(held.Err).NotTo(gomega.HaveOccurred())
		contexts := faultrecovery.MetricSum(s.II, Endorsers[0], faultrecovery.ViewContexts)
		waited := make(chan error, 1)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), viewTimeout)
			defer cancel()
			_, err := s.II.Client(Endorsers[0]).CallViewWithContext(ctx, "finality", common.JSONMarshall(&cviews.Finality{TxID: held.TxID}))
			waited <- err
		}()
		gomega.Eventually(func() float64 {
			return faultrecovery.MetricSum(s.II, Endorsers[0], faultrecovery.ViewContexts)
		}, viewTimeout, 100*time.Millisecond).Should(gomega.BeNumerically(">", contexts), "the finality wait runs on [%s]", Endorsers[0])
		stopped := make(chan struct{})
		go func() {
			defer ginkgo.GinkgoRecover()
			defer close(stopped)
			s.II.StopFSCNode(Endorsers[0])
		}()

		// Round robin asks endorser1 first within len(Endorsers) txs; once it drains, it
		// rejects the session. Until then it may still sign.
		drained := false
		for i := 0; !drained && i < 4*len(Endorsers); i++ {
			sub := submit(s.II, 301+i)
			gomega.Expect(sub.Err).NotTo(gomega.HaveOccurred(), "tx with value %d", sub.Value)
			for _, f := range sub.Failed {
				ginkgo.GinkgoWriter.Printf("tx with value %d: attempt on %s failed (%s): %s\n", sub.Value, f.Endorser, f.Reason, f.Err)
				gomega.Expect(f.Endorser).To(gomega.Equal(Endorsers[0]), "tx with value %d", sub.Value)
				drained = drained || f.Reason == views.Draining
			}
		}
		gomega.Expect(drained).To(gomega.BeTrue(), "alice's attempt on the draining endorser was rejected")

		// Ordering the held tx ends the finality wait, and with it the drain.
		gomega.Expect(order(s.II, &held)).To(gomega.Succeed())
		gomega.Eventually(stopped, viewTimeout).Should(gomega.BeClosed(), "[%s] did not exit", Endorsers[0])
		gomega.Eventually(waited, viewTimeout).Should(gomega.Receive(), "the finality wait on [%s] returned", Endorsers[0])
		s.II.StartFSCNode(Endorsers[0])
		expectSignsAgain(s.II, Endorsers[0])
	})

	// hung is the transaction alice sent to the paused endorser, for the next spec.
	var hung *submission
	ginkgo.It("moves on when an endorser hangs", func() {
		s.II.PauseFSCNode(Endorsers[0])
		ginkgo.DeferCleanup(s.II.ResumeFSCNode, Endorsers[0])
		// Round robin asks the paused endorser first within len(Endorsers) txs.
		for i := 0; hung == nil && i < len(Endorsers); i++ {
			sub := submit(s.II, 100+i)
			gomega.Expect(sub.Err).NotTo(gomega.HaveOccurred(), "tx with value %d", sub.Value)
			if len(sub.Failed) == 0 {
				continue
			}
			// Alice's deadline ends the attempt on the paused endorser.
			gomega.Expect(sub.Failed).To(gomega.HaveExactElements(gomega.And(
				gomega.HaveField("Endorser", Endorsers[0]),
				gomega.HaveField("Reason", views.Deadline),
			)))
			// Alice gave up before the view's one-minute timer would have.
			gomega.Expect(sub.Took).To(gomega.BeNumerically("<", time.Minute))
			hung = &sub
		}
		gomega.Expect(hung).NotTo(gomega.BeNil(), "alice asked the paused endorser")
	})

	// Pending until https://github.com/hyperledger-labs/fabric-smart-client/issues/2618 is
	// fixed: on Fabric, an endorser whose responder marks the tx Busy after its committer
	// skipped the tx's block keeps it Busy.
	ginkgo.PIt("settles a tx on the endorser alice gave up on", func() {
		gomega.Expect(hung).NotTo(gomega.BeNil(), "the previous spec recorded the tx")
		faultrecovery.ExpectSettled(s.II, "alice", hung.TxID, hung.Value)
		faultrecovery.ExpectSettled(s.II, hung.Endorser, hung.TxID, hung.Value)

		// Once resumed, the paused endorser handles alice's request and the tx's block in
		// either order. If its responder marks the tx Busy before the block, the block
		// commits it Valid. If the block comes first, the endorser should keep no record of
		// it once #2618 is fixed; today it keeps the tx Busy, waiting for a block that has
		// passed. Signing again shows it is past that block and has handled alice's request.
		expectSignsAgain(s.II, Endorsers[0])
		settled := func(g gomega.Gomega) {
			got, err := faultrecovery.Query(s.II, Endorsers[0], hung.TxID)
			g.Expect(err).NotTo(gomega.HaveOccurred())
			g.Expect(got.Status).To(gomega.BeElementOf(fdriver.Valid, fdriver.Unknown))
			if got.Status == fdriver.Valid {
				g.Expect(got.Value).To(gomega.Equal(hung.Value))
			}
		}
		gomega.Eventually(settled, faultrecovery.RecoveryTimeout, time.Second).Should(gomega.Succeed(), "[%s] settles tx [%s] or keeps no record of it", Endorsers[0], hung.TxID)
		// A late responder could still mark the tx Busy after the first poll.
		gomega.Consistently(settled, 5*time.Second, time.Second).Should(gomega.Succeed(), "[%s] marked tx [%s] Busy", Endorsers[0], hung.TxID)
	})
}

// submission is the outcome of one submit.
type submission struct {
	views.Endorsed
	Value   int
	Started time.Time
	// Took is how long collecting the endorsement took.
	Took time.Duration
	Err  error
}

// submit has alice create a transaction with value, endorse it with one of Endorsers
// through EndorseAnyView, and order it, and returns the outcome once the transaction is
// final on alice. It reports every failure in Err, so it is safe to call from a goroutine.
func submit(ii *integration.Infrastructure, value int) submission {
	sub := endorse(ii, value, endorsersOf(ii))
	if sub.Err == nil {
		sub.Err = order(ii, &sub)
	}
	return sub
}

// endorse has alice create a transaction with value and endorse it with one of endorsers
// through EndorseAnyView, without ordering it.
func endorse(ii *integration.Infrastructure, value int, endorsers []views.Endorser) submission {
	sub := submission{Value: value, Started: time.Now()}
	ctx, cancel := context.WithTimeout(context.Background(), viewTimeout)
	defer cancel()
	raw, err := ii.Client("alice").CallViewWithContext(ctx, "endorseAny", common.JSONMarshall(&views.EndorseAny{
		Value:          value,
		Endorsers:      endorsers,
		AttemptTimeout: AttemptTimeout,
		Cooldown:       Cooldown,
	}))
	sub.Took = time.Since(sub.Started)
	if err != nil {
		sub.Err = err
		return sub
	}
	sub.Err = json.Unmarshal(raw.([]byte), &sub.Endorsed)
	return sub
}

// order has alice order the transaction sub endorsed and wait until it is final on her.
func order(ii *integration.Infrastructure, sub *submission) error {
	ctx, cancel := context.WithTimeout(context.Background(), viewTimeout)
	defer cancel()
	_, err := ii.Client("alice").CallViewWithContext(ctx, "order", common.JSONMarshall(&fviews.Order{Tx: sub.Tx}))
	return err
}

// expectSignsAgain waits until alice gets a transaction signed by endorser, a node that
// was down and is starting again.
func expectSignsAgain(ii *integration.Infrastructure, endorser string) {
	gomega.EventuallyWithOffset(1, func(g gomega.Gomega) {
		sub := submit(ii, 0)
		g.Expect(sub.Err).NotTo(gomega.HaveOccurred())
		g.Expect(sub.Endorser).To(gomega.Equal(endorser))
	}, rejoinTimeout, time.Second).Should(gomega.Succeed(), "[%s] never signed again", endorser)
}

// underLoad has alice submit one transaction after the other while it runs fault, once
// three have finished, and returns every submission once len(Endorsers)+1 more have
// finished after fault returned: the one in flight then, and a full round robin, so one of
// them asks the endorser fault took down first.
func underLoad(ii *integration.Infrastructure, fault func()) []submission {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var done atomic.Int32
	subs := make(chan []submission, 1)
	go func() {
		var all []submission
		for i := 0; ctx.Err() == nil; i++ {
			all = append(all, submit(ii, 200+i)) //nolint:contextcheck // ctx only says when to stop
			done.Add(1)
		}
		subs <- all
	}()
	gomega.EventuallyWithOffset(1, done.Load, viewTimeout, 100*time.Millisecond).Should(gomega.BeNumerically(">=", 3))
	fault()
	after := done.Load()
	gomega.EventuallyWithOffset(1, done.Load, viewTimeout, 100*time.Millisecond).Should(gomega.BeNumerically(">=", after+int32(len(Endorsers))+1))
	cancel()
	return <-subs
}

// endorsersOf returns Endorsers with their identities in ii.
func endorsersOf(ii *integration.Infrastructure) []views.Endorser {
	es := make([]views.Endorser, len(Endorsers))
	for i, name := range Endorsers {
		es[i] = views.Endorser{Name: name, Identity: ii.Identity(name)}
	}
	return es
}
