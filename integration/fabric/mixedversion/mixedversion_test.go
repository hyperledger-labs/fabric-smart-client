/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mixedversion_test

import (
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/mixedversion"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/mixedversion/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

// previousRelease is the FSC release the specs run some nodes at: a module version, such
// as v0.28.0, or the absolute path of a checkout. The Makefile sets it to the latest
// release tag.
var previousRelease = os.Getenv("FSC_PREVIOUS_RELEASE")

// The specs need the views of this suite at the previous release, because the nodes of that
// release run its copy of them; a release older than the suite is skipped.
var _ = BeforeEach(func() {
	Expect(previousRelease).NotTo(BeEmpty(), "set FSC_PREVIOUS_RELEASE")
	ok, err := fsc.ReleaseHasPackage(previousRelease, "fabric/mixedversion/views")
	Expect(err).NotTo(HaveOccurred())
	if !ok {
		Skip("FSC release " + previousRelease + " predates the mixed-version suite")
	}
})

// newSuite returns a suite on mixedversion.Topology whose nodes in previous run the previous
// release; the others run the current tree.
func newSuite(previous ...string) *integration.TestSuite {
	return integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		ii, err := integration.Generate(integration.MixedVersionPort.StartPortForNode(), integration.WithRaceDetection, mixedversion.Topology()...)
		if err != nil {
			return nil, err
		}
		for _, id := range previous {
			ii.FscPlatform.SetRelease(id, previousRelease)
		}
		return ii, nil
	})
}

var _ = Describe("Mixed versions", Label(fsc.WebSocket), func() {
	Describe("an approver on the previous release", func() {
		s := newSuite("approver")
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
		s := newSuite("alice")
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)

		// alice's replicas share one database, which the previous release created. One
		// replica at a time moves to the current tree and back, as an operator's rolling
		// upgrade and rollback do, while the other keeps serving the same stores. Every
		// write stores state metadata, and every update reads the metadata the other
		// release wrote: the current tree holds its contract steps back on these stores,
		// so both releases keep writing the form the previous release reads.
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
	})
})

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

// expectValue waits until replica reads value for the asset id. Both replicas of alice
// commit every block to their shared database, so the other replica's commit may land
// first or last.
func expectValue(ii *integration.Infrastructure, replica, id string, value int) {
	Eventually(func(g Gomega) {
		res, err := ii.Client(replica).CallView("query", common.JSONMarshall(&views.Query{ID: id}))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(common.JSONUnmarshalInt(res)).To(Equal(value))
	}, time.Minute, time.Second).Should(Succeed())
}
