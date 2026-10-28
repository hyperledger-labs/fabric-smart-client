/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iou_test

import (
	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
)

var _ = Describe("EndToEnd", func() {
	// Both transports run the basic life cycle: this suite is the p2p-heavy
	// end-to-end case (4 FSC nodes exchanging recipient identities and
	// collecting endorsements), so it is where libp2p is worth its cost.
	for _, c := range []fsc.P2PCommunicationType{fsc.WebSocket, fsc.LibP2P} {
		Describe("IOU Life Cycle", Label(c), func() {
			s := NewTestSuite(c, integration.NoReplication)
			BeforeEach(s.Setup)
			AfterEach(s.TearDown)
			It("creates and updates an IOU with approvers and records its metrics and traces", s.TestSucceeded)
		})
	}

	// The replicas variant varies replication, not the transport, so it runs on websocket only.
	// It still carries the label: a --label-filter selects only labelled specs, so an unlabelled
	// Describe would be skipped. The replicas also cover the storage options of nwo: the
	// borrower prefixes its tables and keeps its KVS in a second database, the lender connects
	// over TLS, and the approvers keep the SQLite defaults.
	Describe("IOU Life Cycle With Websockets and replicas", Label(fsc.WebSocket), func() {
		s := NewTestSuite(
			fsc.WebSocket,
			&integration.ReplicationOptions{
				ReplicationFactors: map[string]int{
					"borrower": 3,
					"lender":   2,
				},
				SQLConfigs: map[string]*postgres.ContainerConfig{
					"borrower": postgres.DefaultConfig(postgres.WithDBName("borrower-db")),
					"lender":   postgres.DefaultConfig(postgres.WithDBName("lender-db")),
				},
				NodeOptions: map[string][]node.Option{
					"borrower": {
						fabric.WithTablePrefix(common.DefaultPersistence, "borrower"),
						fabric.WithPostgresPersistence(borrowerKVS, postgres.DefaultConfig(postgres.WithDBName("borrower-kvs-db"))),
						fabric.WithStorePersistence("fsc_kvs", borrowerKVS),
					},
					"lender": {fabric.WithPostgresTLS(common.DefaultPersistence)},
				},
			},
		)
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)
		It("creates and updates an IOU across replicas, in the configured databases and over TLS", s.TestSucceededWithReplicas)
	})
})

type TestSuite struct {
	*integration.TestSuite
}

func NewTestSuite(commType fsc.P2PCommunicationType, nodeOpts *integration.ReplicationOptions) *TestSuite {
	return &TestSuite{TestSuite: integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		return integration.Generate(StartPort(), integration.WithRaceDetection, iou.Topology(&iou.Opts{
			CommType:        commType,
			ReplicationOpts: nodeOpts,
		})...)
	})}
}

func (s *TestSuite) TestSucceeded() {
	iou.InitApprover(s.II, "approver1")
	iou.InitApprover(s.II, "approver2")
	iouState := iou.CreateIOU(s.II, "", 10, "approver1")
	iou.CheckState(s.II, "borrower", iouState, 10)
	iou.CheckState(s.II, "lender", iouState, 10)
	iou.UpdateIOU(s.II, iouState, 5, "approver2")
	iou.CheckState(s.II, "borrower", iouState, 5)
	iou.CheckState(s.II, "lender", iouState, 5)

	iou.CheckLocalMetrics(s.II, "borrower", "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser/collectEndorsementsView")
	iou.CheckPrometheusMetrics(s.II, "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser/collectEndorsementsView")
	iou.CheckJaegerTraces(s.II, "borrower", "", gomega.And(
		gomega.Not(gomega.BeNil()),
		gomega.Not(gomega.BeEmpty()),
	))
}

func (s *TestSuite) TestSucceededWithReplicas() {
	iou.InitApprover(s.II, "approver1")
	iou.InitApprover(s.II, "approver2")

	iouState := iou.CreateIOUWithBorrower(s.II, "fsc.borrower.0", "", 10, "approver1")
	iou.CheckState(s.II, "fsc.borrower.0", iouState, 10)
	iou.CheckState(s.II, "fsc.borrower.1", iouState, 10)
	iou.CheckState(s.II, "fsc.borrower.2", iouState, 10)
	iou.CheckState(s.II, "fsc.lender.0", iouState, 10)
	iou.CheckState(s.II, "fsc.lender.1", iouState, 10)

	iou.UpdateIOUWithBorrower(s.II, "fsc.borrower.1", iouState, 5, "approver2")
	iou.CheckState(s.II, "fsc.borrower.0", iouState, 5)
	iou.CheckState(s.II, "fsc.borrower.1", iouState, 5)
	iou.CheckState(s.II, "fsc.borrower.2", iouState, 5)
	iou.CheckState(s.II, "fsc.lender.0", iouState, 5)
	iou.CheckState(s.II, "fsc.lender.1", iouState, 5)

	// The borrower's vault and other stores are in its prefixed default database, its KVS in
	// the second one.
	borrowerTables := iou.Tables(s.II, "borrower.0", common.DefaultPersistence)
	gomega.Expect(borrowerTables).To(gomega.ContainElement(gomega.HavePrefix("borrower_default__testchannel_vstate")))
	gomega.Expect(borrowerTables).To(gomega.HaveEach(gomega.HavePrefix("borrower_")))
	gomega.Expect(borrowerTables).NotTo(gomega.ContainElement(gomega.HaveSuffix("_kvs")))
	gomega.Expect(iou.Tables(s.II, "borrower.0", borrowerKVS)).To(gomega.ConsistOf("fsc_kvs"))
	gomega.Expect(iou.Tables(s.II, "lender.0", common.DefaultPersistence)).To(gomega.ContainElement("fsc_kvs"))

	// The lender's database also accepts plaintext, so check that the lender's replicas, the
	// only clients of that database, connect over TLS.
	total, tls := iou.Connections(s.II, "lender.0", common.DefaultPersistence)
	gomega.Expect(total).To(gomega.BeNumerically(">", 0))
	gomega.Expect(tls).To(gomega.Equal(total), "every lender connection uses TLS")
}

// borrowerKVS is the persistence that holds the borrower's KVS in the replicas variant.
const borrowerKVS = "kvs"
