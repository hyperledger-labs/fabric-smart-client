/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mixedversion_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/mixedversion"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/mixedversion/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/runner"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/node/start"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
)

// previousRelease is the FSC release the specs run some nodes at: a module version, such
// as v0.28.0, or the absolute path of a checkout. The Makefile sets it to the latest
// release tag.
var previousRelease = os.Getenv("FSC_PREVIOUS_RELEASE")

// requirePreviousRelease checks that the previous release has the views of this suite,
// because the nodes of that release run its copy of them. A release older than the suite fails
// every spec that calls it, rather than with a build error once a node starts, and never passes
// as skipped.
func requirePreviousRelease() {
	Expect(previousRelease).NotTo(BeEmpty(), "set FSC_PREVIOUS_RELEASE")
	ok, err := fsc.ReleaseHasPackage(previousRelease, "fabric/mixedversion/views")
	Expect(err).NotTo(HaveOccurred())
	Expect(ok).To(BeTrue(), "FSC release "+previousRelease+" predates the mixed-version suite")
}

// newSuite returns a suite on topology whose nodes in previous run the previous release;
// the others run the current tree.
func newSuite(topology func() []api.Topology, previous ...string) *integration.TestSuite {
	return newSuiteAt(topology, previousRelease, previous...)
}

// newSuiteAt returns a suite on topology whose nodes in ids run release; the others run the
// current tree.
func newSuiteAt(topology func() []api.Topology, release string, ids ...string) *integration.TestSuite {
	return integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		ii, err := integration.Generate(integration.MixedVersionPort.StartPortForNode(), integration.WithRaceDetection, topology()...)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			ii.FscPlatform.SetRelease(id, release)
		}
		return ii, nil
	})
}

var _ = Describe("Mixed versions", Label(fsc.WebSocket), func() {
	BeforeEach(requirePreviousRelease)

	Describe("an approver on the previous release", func() {
		s := newSuite(mixedversion.Topology, "approver")
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)

		// The initiator on the current tree negotiates with an approver on the previous
		// release, which endorses in the formats both support.
		It("endorses the transactions of an initiator on the current tree", func() {
			create(s.II, "fsc.alice.0", "a1", 10, contract)
			expectValue(s.II, "fsc.alice.1", "a1", 10)
			update(s.II, "fsc.alice.0", "a1", 5)
			expectValue(s.II, "fsc.alice.1", "a1", 5)

			session := s.II.RunFSCNodeCommand("alice.0", "node", "peers", "--check")
			Expect(session).To(gexec.Exit(0), string(session.Err.Contents()))
			Expect(string(session.Out.Contents())).To(ContainSubstring("peer [approver]"))
		})
	})

	Describe("a node upgraded replica by replica", func() {
		s := newSuite(mixedversion.Topology, "alice")
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)

		// alice's replicas share one database, which the previous release created. One
		// replica at a time moves to the current tree and back, as an operator's rolling
		// upgrade and rollback do, while the other keeps serving the same stores. Every
		// write stores state metadata, and every update reads the metadata the other
		// release wrote: the current tree holds back every contract step the previous
		// release lacks, so both releases keep writing a form the other reads.
		It("keeps reading and updating its stores across the upgrade and a rollback", func() {
			create(s.II, "fsc.alice.0", "a1", 10, contract)

			s.II.UpgradeFSCNode("alice.1", "")
			expectValue(s.II, "fsc.alice.1", "a1", 10)
			update(s.II, "fsc.alice.1", "a1", 9)
			expectValue(s.II, "fsc.alice.0", "a1", 9)
			update(s.II, "fsc.alice.0", "a1", 8)
			expectValue(s.II, "fsc.alice.1", "a1", 8)
			update(s.II, "fsc.alice.1", "a1", 7)

			s.II.UpgradeFSCNode("alice.1", previousRelease)
			expectValue(s.II, "fsc.alice.1", "a1", 7)
			update(s.II, "fsc.alice.1", "a1", 6)

			s.II.UpgradeFSCNode("alice.0", "")
			s.II.UpgradeFSCNode("alice.1", "")
			update(s.II, "fsc.alice.0", "a1", 5)
			expectValue(s.II, "fsc.alice.1", "a1", 5)
		})

		// alice's replicas commit every block to their shared database, so a replica that
		// dies after it broadcast a transaction leaves it Busy there for the replica of the
		// other release to settle. alice.1 moves to the current tree, and each replica in
		// turn endorses, broadcasts and is killed. Org2's peer, which delivers blocks to both
		// replicas, is down from the broadcast on, so the transaction is ordered but no replica commits
		// it before the kill: the survivor reads the shared database and finds no asset.
		// Once the peer is back, the survivor commits the transaction as valid, the only
		// way it reads the asset, and the restarted replica spends it.
		It("settles the transaction of a killed replica on the replica of the other release", func() {
			s.II.UpgradeFSCNode("alice.1", "")
			for i, r := range [][2]string{{"alice.0", "alice.1"}, {"alice.1", "alice.0"}} {
				killed, survivor := "fsc."+r[0], "fsc."+r[1]
				id := fmt.Sprintf("k%d", i)
				res, err := s.II.Client(killed).CallView("endorse", common.JSONMarshall(&views.Create{
					ID: id, Value: 10, Contract: contract, Approver: s.II.Identity("approver"),
				}))
				Expect(err).NotTo(HaveOccurred())
				s.II.StopPeer("Org2_peer_0")
				_, err = s.II.Client(killed).CallView("broadcast", common.JSONMarshall(&views.Broadcast{Tx: res.([]byte)}))
				Expect(err).NotTo(HaveOccurred())
				s.II.KillFSCNode(r[0])
				Consistently(func() error {
					_, err := s.II.Client(survivor).CallView("query", common.JSONMarshall(&views.Query{ID: id}))
					return err
				}, 3*time.Second, time.Second).Should(MatchError(ContainSubstring("not found")), "a replica committed [%s] before the kill", id)

				s.II.StartPeer("Org2_peer_0")
				expectValue(s.II, survivor, id, 10)
				s.II.StartFSCNode(r[0])
				update(s.II, killed, id, 9)
				expectValue(s.II, survivor, id, 9)
			}
		})

		// The contract-step runbook of the upgrade guides on alice's shared database. The
		// previous release creates the stores, which runs every contract step it knows, so
		// the steps under test are those `version --json` lists for the current tree only,
		// in store types both releases have; a store type only the current tree has is new
		// and runs its contract steps at its first open. `node migrate --contract` waits
		// for these steps while alice's opener rows of the previous release remain, and
		// once they ran, the previous release refuses the stores. If the previous release
		// knows every such step, nothing waits and a rollback reads the stores.
		It("runs its contract steps once no replica runs the previous release", func() {
			create(s.II, "fsc.alice.0", "a1", 10, contract)
			previous := nodeCommand(s.II, "alice.1")
			previousSteps := contractSteps(s.II.RunFSCNodeCommand("alice.1", "version", "--json"))

			// alice's opener rows of the previous release, among them alice.0's, block each
			// step that release lacks, and --contract names alice.
			s.II.UpgradeFSCNode("alice.1", "")
			var newSteps []string
			for store, steps := range contractSteps(s.II.RunFSCNodeCommand("alice.1", "version", "--json")) {
				if previous, ok := previousSteps[store]; ok {
					for _, step := range steps {
						if !slices.Contains(previous, step) {
							newSteps = append(newSteps, store+"/"+step)
						}
					}
				}
			}
			GinkgoWriter.Printf("contract steps the previous release lacks: %v\n", newSteps)
			s.II.StopFSCNode("alice.1")
			if len(newSteps) > 0 {
				out := expectMigrate(s.II, start.ExitOlderOpener, "alice.1", "--check", "--contract")
				Expect(out).To(ContainSubstring("node [alice]"))
			} else {
				expectMigrate(s.II, 0, "alice.1", "--check", "--contract")
			}
			s.II.StartFSCNode("alice.1")
			update(s.II, "fsc.alice.1", "a1", 9)
			expectValue(s.II, "fsc.alice.0", "a1", 9)

			// With both replicas on the current tree, forgetting alice's opener rows lets
			// the steps run, and none is pending afterwards.
			s.II.UpgradeFSCNode("alice.0", "")
			s.II.StopFSCNode("alice.0")
			s.II.StopFSCNode("alice.1")
			expectMigrate(s.II, 0, "alice.1", "--contract", "--forget-opener", "alice")
			expectMigrate(s.II, 0, "alice.1", "--check", "--contract")
			s.II.StartFSCNode("alice.0")
			s.II.StartFSCNode("alice.1")
			expectValue(s.II, "fsc.alice.1", "a1", 9)
			update(s.II, "fsc.alice.1", "a1", 8)
			expectValue(s.II, "fsc.alice.0", "a1", 8)

			// Rolling alice.1 back: a release without a step that ran refuses the stores, which
			// its `node migrate --check` reports before a start would; one that knows every
			// step reads and updates them.
			if len(newSteps) > 0 {
				s.II.StopFSCNode("alice.1")
				cmd := exec.Command(previous.Path, "node", "migrate", "--check")
				cmd.Env, cmd.Dir = previous.Env, previous.Dir
				session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())
				Eventually(session, time.Minute).Should(gexec.Exit(start.ExitSchemaTooNew))
				Expect(string(session.Err.Contents())).To(ContainSubstring(migrate.ErrSchemaTooNew.Error()))
				return
			}
			s.II.UpgradeFSCNode("alice.1", previousRelease)
			expectValue(s.II, "fsc.alice.1", "a1", 8)
			update(s.II, "fsc.alice.1", "a1", 7)
			expectValue(s.II, "fsc.alice.0", "a1", 7)
		})
	})

	Describe("a responder upgraded replica by replica under load", func() {
		s := newSuite(mixedversion.RollingTopology, "approver")
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)

		// alice submits one transaction after the other while the approver's replicas, behind
		// a load balancer on one database, move to the current tree one at a time and one moves
		// back. A stopping replica drains: alice's session in progress finishes, and her next
		// one gets "draining", which makes her dial again through the balancer and reach a
		// replica that serves. So at most one submission fails per restart, and a failed
		// submission must not commit: alice got an error, so a committed asset would be a
		// success she never saw.
		It("keeps committing through the upgrade and a rollback", func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var committed atomic.Int32
			done := make(chan []submission, 1)
			go func() {
				defer GinkgoRecover()
				done <- submitUntil(ctx, s.II, &committed)
			}()

			// Each step restarts the replica that holds alice's connection, since restarting the
			// idle one tests nothing: the first two upgrade both replicas, because alice moves to
			// the other one after each drain, and the last rolls one back.
			releases := []string{"", "", previousRelease}
			var last string
			for _, release := range releases {
				before := committed.Load()
				Eventually(committed.Load, time.Minute, time.Second).Should(BeNumerically(">", before+1), "no commits before a restart")
				held, _ := faultrecovery.ConnectedReplica(s.II, "approver")
				Expect(held.UniqueName).NotTo(Equal(last), "alice stayed on the restarted replica")
				last = held.UniqueName
				s.II.UpgradeFSCNode(held.UniqueName, release)
			}
			before := committed.Load()
			Eventually(committed.Load, time.Minute, time.Second).Should(BeNumerically(">", before+1), "no commits after the last restart")
			cancel()
			var subs []submission
			Eventually(done, 3*time.Minute).Should(Receive(&subs), "submissions did not stop")

			// The count comes first, so a run of failures fails the spec at once instead of
			// checking each failed submission for 5s.
			var failed []submission
			for _, sub := range subs {
				if sub.err != nil {
					GinkgoWriter.Printf("submission [%s] failed: %v\n", sub.id, sub.err)
					failed = append(failed, sub)
				}
			}
			Expect(len(failed)).To(BeNumerically("<=", len(releases)), "at most one failed submission per restart")
			for _, sub := range failed {
				Consistently(func() error {
					_, err := s.II.Client("fsc.approver.0").CallView("query", common.JSONMarshall(&views.Query{ID: sub.id}))
					return err
				}, 5*time.Second, time.Second).Should(MatchError(ContainSubstring("not found")), "failed submission [%s] committed", sub.id)
			}
			for i, sub := range subs {
				if sub.err == nil {
					for _, replica := range []string{"fsc.approver.0", "fsc.approver.1", "fsc.alice.0"} {
						expectValue(s.II, replica, sub.id, i)
					}
				}
			}
		})
	})
})

// submission is the outcome of creating the asset id.
type submission struct {
	id  string
	err error
}

// submitUntil creates the assets r0, r1, ... with values 0, 1, ... from alice, one after
// the other, until ctx ends, and returns every submission in order, so the i-th has value i. It counts each that
// committed in committed. A submission in progress when ctx ends runs to completion.
func submitUntil(ctx context.Context, ii *integration.Infrastructure, committed *atomic.Int32) []submission {
	var subs []submission
	for i := 0; ctx.Err() == nil; i++ {
		sub := submission{id: fmt.Sprintf("r%d", i)}
		// ctx only says when to stop: cancelling a submission in progress would record a
		// failure the spec did not cause.
		callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		_, sub.err = ii.Client("alice").CallViewWithContext(callCtx, "create", common.JSONMarshall(&views.Create{
			ID: sub.id, Value: i, Approver: ii.Identity("approver"),
		}))
		cancel()
		if sub.err == nil {
			committed.Add(1)
		}
		subs = append(subs, sub)
	}
	return subs
}

// nodeCommand returns the start command of the FSC replica id, such as alice.1, whose
// binary and environment run that replica at its current release.
func nodeCommand(ii *integration.Infrastructure, id string) *exec.Cmd {
	for _, m := range ii.NWO.ViewMembers {
		if m.Name == "fsc."+id {
			return m.Runner.(*runner.Runner).Command
		}
	}
	Fail("no FSC replica " + id)
	return nil
}

// contractSteps returns the contract steps of each Postgres store type in the output of
// `version --json`, as "<version> <name>"; a store type without contract steps maps to none.
func contractSteps(session *gexec.Session) map[string][]string {
	ExpectWithOffset(1, session).To(gexec.Exit(0), string(session.Err.Contents()))
	var report struct {
		Stores []struct {
			Type    string
			Dialect string
			Steps   []struct {
				Version  int
				Name     string
				Contract bool
			}
		}
	}
	ExpectWithOffset(1, json.Unmarshal(session.Out.Contents(), &report)).To(Succeed())
	steps := map[string][]string{}
	for _, st := range report.Stores {
		if st.Dialect != string(migrate.Postgres) {
			continue
		}
		steps[st.Type] = []string{}
		for _, step := range st.Steps {
			if step.Contract {
				steps[st.Type] = append(steps[st.Type], fmt.Sprintf("%d %s", step.Version, step.Name))
			}
		}
	}
	return steps
}

// expectMigrate runs `node migrate` with flags for the stopped replica id, expects it to
// exit with code and returns its output.
func expectMigrate(ii *integration.Infrastructure, code int, id string, flags ...string) string {
	session := ii.MigrateFSCNode(id, flags...)
	out := string(session.Out.Contents()) + string(session.Err.Contents())
	ExpectWithOffset(1, session.ExitCode()).To(Equal(code), out)
	return out
}

// contract is stored in the state metadata of every asset the specs write.
const contract = "mixedversion"

func create(ii *integration.Infrastructure, replica, id string, value int, contract string) {
	_, err := ii.Client(replica).CallView("create", common.JSONMarshall(&views.Create{
		ID: id, Value: value, Contract: contract, Approver: ii.Identity("approver"),
	}))
	Expect(err).NotTo(HaveOccurred())
}

func update(ii *integration.Infrastructure, replica, id string, value int) {
	_, err := ii.Client(replica).CallView("update", common.JSONMarshall(&views.Update{
		ID: id, Value: value, Contract: contract, Approver: ii.Identity("approver"),
	}))
	Expect(err).NotTo(HaveOccurred())
}

// expectValue waits until replica reads value for the asset id. The replicas of a node
// commit every block to their shared database, so another replica's commit may land
// first or last.
func expectValue(ii *integration.Infrastructure, replica, id string, value int) {
	Eventually(func(g Gomega) {
		res, err := ii.Client(replica).CallView("query", common.JSONMarshall(&views.Query{ID: id}))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(common.JSONUnmarshalInt(res)).To(Equal(value))
	}, time.Minute, time.Second).Should(Succeed())
}
