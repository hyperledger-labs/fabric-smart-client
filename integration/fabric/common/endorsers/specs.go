/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorsers

import (
	"context"
	"encoding/json"
	"os"
	"sync/atomic"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/endorsers/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery"
	fviews "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

const (
	// Attempt bounds alice's attempt on one endorser. A healthy endorser answers well
	// within it, and a hung one costs alice Attempt instead of the view's one-minute timer.
	Attempt = 5 * time.Second
	// Cooldown is how long alice asks an endorser that failed only after the others.
	Cooldown = 10 * time.Second
	// viewTimeout bounds each view call, so a node that never answers fails the spec
	// instead of hanging it.
	viewTimeout = 2 * time.Minute
	// rejoinTimeout bounds how long a restarted endorser takes to sign again: it has to
	// start, and alice asks it first again only once Cooldown has passed.
	rejoinTimeout = Cooldown + time.Minute
)

// Specs registers the specs of an endorsers suite for s, a suite on FSCTopology under
// faultrecovery.ShareNetwork; its Describe must be Ordered. Each spec leaves every node
// running. The last one checks the transaction the spec before it sent to the endorser it
// paused, so the two run in that order.
func Specs(s *integration.TestSuite) {
	ginkgo.It("spreads endorsements over every endorser", func() {
		signed := map[string]int{}
		for i := range 30 {
			sub := Submit(s.II, i)
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
		ExpectSignsAgain(s.II, Endorsers[0])
	})

	ginkgo.It("moves on when an endorser drains", func() {
		// StopFSCNode returns only once the node has exited, so the submissions run while
		// it drains and after it is gone.
		subs := underLoad(s.II, func() { s.II.StopFSCNode(Endorsers[0]) })
		var failed []string
		for _, sub := range subs {
			gomega.Expect(sub.Err).NotTo(gomega.HaveOccurred(), "tx with value %d", sub.Value)
			for _, a := range sub.Failed {
				gomega.Expect(a.Endorser).To(gomega.Equal(Endorsers[0]), "tx with value %d", sub.Value)
				ginkgo.GinkgoWriter.Printf("tx with value %d: attempt on %s failed: %s\n", sub.Value, a.Endorser, a.Err)
				failed = append(failed, a.Err)
			}
		}
		gomega.Expect(failed).NotTo(gomega.BeEmpty(), "alice asked the stopped endorser")
		gomega.Expect(failed).To(gomega.HaveEach(gomega.Or(
			gomega.ContainSubstring("remote rejection: draining"),
			gomega.ContainSubstring("connection refused"),
		)), "an attempt on a draining endorser is rejected, and one on an exited endorser cannot dial")
		s.II.StartFSCNode(Endorsers[0])
		ExpectSignsAgain(s.II, Endorsers[0])
	})

	// hung is the transaction alice sent to the paused endorser, for the next spec.
	var hung *Submission
	ginkgo.It("moves on when an endorser hangs", func() {
		s.II.PauseFSCNode(Endorsers[0])
		ginkgo.DeferCleanup(s.II.ResumeFSCNode, Endorsers[0])
		// Round robin asks the paused endorser first within len(Endorsers) txs.
		for i := 0; hung == nil && i < len(Endorsers); i++ {
			sub := Submit(s.II, 100+i)
			gomega.Expect(sub.Err).NotTo(gomega.HaveOccurred(), "tx with value %d", sub.Value)
			if len(sub.Failed) == 0 {
				continue
			}
			gomega.Expect(sub.Failed).To(gomega.HaveExactElements(gomega.HaveField("Endorser", Endorsers[0])))
			// The deadline ends the attempt in the open session, or in the handshake of a new
			// connection to the paused endorser.
			gomega.Expect(sub.Failed[0].Err).To(gomega.Or(
				gomega.ContainSubstring(context.DeadlineExceeded.Error()),
				gomega.ContainSubstring(os.ErrDeadlineExceeded.Error()),
			))
			// Alice gave up at her deadline, well before the view's one-minute timer.
			gomega.Expect(sub.Took).To(gomega.BeNumerically("<", Attempt+20*time.Second))
			hung = &sub
		}
		gomega.Expect(hung).NotTo(gomega.BeNil(), "alice asked the paused endorser")
		s.II.ResumeFSCNode(Endorsers[0])
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
		// commits it Valid. If the block comes first, the endorser has no record of the tx
		// and reads it as one another endorser signed; its responder, which runs in the
		// context of alice's ended session, stores and sends nothing. It must never keep
		// the tx Busy, waiting for a block that has passed. Signing again shows it is past
		// that block.
		ExpectSignsAgain(s.II, Endorsers[0])
		other := SubmitSignedByOther(s.II, Endorsers[0])
		gomega.Eventually(func(g gomega.Gomega) {
			got, err := faultrecovery.Query(s.II, Endorsers[0], hung.TxID)
			g.Expect(err).NotTo(gomega.HaveOccurred())
			want, err := faultrecovery.Query(s.II, Endorsers[0], other.TxID)
			g.Expect(err).NotTo(gomega.HaveOccurred())
			g.Expect(got.Status).NotTo(gomega.Equal(fdriver.Busy))
			if got.Status == fdriver.Valid {
				g.Expect(got.Value).To(gomega.Equal(hung.Value))
				return
			}
			g.Expect(got.Status).To(gomega.Equal(want.Status))
		}, faultrecovery.RecoveryTimeout, time.Second).Should(gomega.Succeed(), "[%s] settles tx [%s] or keeps no record of it", Endorsers[0], hung.TxID)
	})
}

// Submission is the outcome of one Submit.
type Submission struct {
	views.Endorsed
	Value   int
	Started time.Time
	// Took is how long collecting the endorsement took.
	Took time.Duration
	Err  error
}

// Submit has alice create a transaction with value, endorse it with one of Endorsers
// through EndorseAnyView, and order it, and returns the outcome once the transaction is
// final on alice. It reports every failure in Err, so it is safe to call from a goroutine.
func Submit(ii *integration.Infrastructure, value int) Submission {
	sub := Submission{Value: value, Started: time.Now()}
	ctx, cancel := context.WithTimeout(context.Background(), viewTimeout)
	defer cancel()
	raw, err := ii.Client("alice").CallViewWithContext(ctx, "endorseAny", common.JSONMarshall(&views.EndorseAny{
		Value:     value,
		Endorsers: endorsersOf(ii),
		Attempt:   Attempt,
		Cooldown:  Cooldown,
	}))
	sub.Took = time.Since(sub.Started)
	if err != nil {
		sub.Err = err
		return sub
	}
	if sub.Err = json.Unmarshal(raw.([]byte), &sub.Endorsed); sub.Err != nil {
		return sub
	}
	_, sub.Err = ii.Client("alice").CallViewWithContext(ctx, "order", common.JSONMarshall(&fviews.Order{Tx: sub.Tx}))
	return sub
}

// SubmitSignedByOther submits transactions until one is signed by an endorser other than
// endorser, and returns it.
func SubmitSignedByOther(ii *integration.Infrastructure, endorser string) Submission {
	for range len(Endorsers) {
		sub := Submit(ii, 0)
		gomega.ExpectWithOffset(1, sub.Err).NotTo(gomega.HaveOccurred())
		if sub.Endorser != endorser {
			return sub
		}
	}
	ginkgo.Fail("every tx was signed by [" + endorser + "]")
	return Submission{}
}

// ExpectSignsAgain waits until alice gets a transaction signed by endorser, a node that
// was down and is starting again.
func ExpectSignsAgain(ii *integration.Infrastructure, endorser string) {
	gomega.EventuallyWithOffset(1, func(g gomega.Gomega) {
		sub := Submit(ii, 0)
		g.Expect(sub.Err).NotTo(gomega.HaveOccurred())
		g.Expect(sub.Endorser).To(gomega.Equal(endorser))
	}, rejoinTimeout, time.Second).Should(gomega.Succeed(), "[%s] never signed again", endorser)
}

// underLoad has alice submit one transaction after the other while it runs fault, once
// three have finished, and returns every submission once three more have finished after
// fault returned.
func underLoad(ii *integration.Infrastructure, fault func()) []Submission {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var done atomic.Int32
	subs := make(chan []Submission, 1)
	go func() {
		var all []Submission
		for i := 0; ctx.Err() == nil; i++ {
			all = append(all, Submit(ii, 200+i)) //nolint:contextcheck // ctx only says when to stop
			done.Add(1)
		}
		subs <- all
	}()
	gomega.EventuallyWithOffset(1, done.Load, viewTimeout, 100*time.Millisecond).Should(gomega.BeNumerically(">=", 3))
	fault()
	after := done.Load()
	gomega.EventuallyWithOffset(1, done.Load, viewTimeout, 100*time.Millisecond).Should(gomega.BeNumerically(">=", after+3))
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
