/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package faultrecovery

import (
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery/views"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

// recoveryTimeout bounds how long a restarted node may take to settle a transaction.
const recoveryTimeout = 2 * time.Minute

// CrashSpecs registers the node crash and catch-up specs for the suite s. They check
// bob through QueryView: on Fabric that reads bob's own vault, so the specs prove that
// bob recovers its local state; on Fabric-x, where bob has no local ledger state, they
// prove only that bob restarts and serves views again.
func CrashSpecs(s *integration.TestSuite) {
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
			}, recoveryTimeout, time.Second).Should(gomega.Succeed(), "tx [%s] not settled on [%s]", txID, node)
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
			}, recoveryTimeout, time.Second).Should(gomega.Succeed(), "tx %d [%s] not caught up on bob", i, txID)
		}
	})
}

// DependencySpecs registers the specs that take down a dependency both suites share: bob's
// Postgres database, named DBName(dbPrefix, "bob").
func DependencySpecs(s *integration.TestSuite, dbPrefix string) {
	ginkgo.It("starts before Postgres is up", func() {
		db := DBName(dbPrefix, "bob")
		s.II.StopFSCNode("bob")
		s.II.StopPostgres(db)

		// StartFSCNode returns only once bob has started, which it cannot do without its
		// database, so the database comes back from another goroutine.
		started := make(chan struct{})
		go func() {
			defer ginkgo.GinkgoRecover()
			defer close(started)
			time.Sleep(10 * time.Second)
			s.II.StartPostgres(db)
		}()
		s.II.StartFSCNode("bob")
		<-started

		gomega.Eventually(s.II.WebClient("bob").Ready, recoveryTimeout, time.Second).Should(gomega.Succeed())
		gomega.Expect(SubmitAndWaitFinal(s.II, 1)).To(gomega.HaveLen(1))
	})
}
