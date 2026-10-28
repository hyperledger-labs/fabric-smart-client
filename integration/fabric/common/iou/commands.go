/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iou

import (
	"database/sql"
	"fmt"
	"time"

	model "github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
	"github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/iou/views"
	cviews "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
)

const (
	ViewCallsOperationsMetric = "fsc_view_services_view_calls_operations"

	// prometheusScrapeEventually bounds the wait for Prometheus to publish a scraped
	// metric. It must comfortably exceed the 15s scrape interval configured in
	// integration/nwo/monitoring/monitoring/monitoring.go.
	prometheusScrapeEventually = 60 * time.Second
)

var logger = logging.MustGetLogger()

func CreateIOU(ii *integration.Infrastructure, identityLabel string, amount uint, approver string) string {
	return CreateIOUWithBorrower(ii, "borrower", identityLabel, amount, approver)
}

func CreateIOUWithBorrower(ii *integration.Infrastructure, borrower, identityLabel string, amount uint, approver string) string {
	res, err := ii.Client(borrower).CallView(
		"create", common.JSONMarshall(&views.Create{
			Amount:   amount,
			Identity: identityLabel,
			Lender:   ii.Identity("lender"),
			Approver: ii.Identity(approver),
		}),
	)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(res).NotTo(gomega.BeNil())
	return common.JSONUnmarshalString(res)
}

func CheckState(ii *integration.Infrastructure, partyID, iouStateID string, expected int) {
	res, err := ii.CLI(partyID).CallView("query", common.JSONMarshall(&views.Query{LinearID: iouStateID}))
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(common.JSONUnmarshalInt(res)).To(gomega.BeEquivalentTo(expected))
}

func UpdateIOU(ii *integration.Infrastructure, iouStateID string, amount uint, approver string) {
	UpdateIOUWithBorrower(ii, "borrower", iouStateID, amount, approver)
}

func UpdateIOUWithBorrower(ii *integration.Infrastructure, borrower, iouStateID string, amount uint, approver string) {
	txIDBoxed, err := ii.Client(borrower).CallView("update",
		common.JSONMarshall(&views.Update{
			LinearID: iouStateID,
			Amount:   amount,
			Approver: ii.Identity(approver),
		}),
	)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	txID := common.JSONUnmarshalString(txIDBoxed)
	_, err = ii.Client("lender").CallView("finality", common.JSONMarshall(cviews.Finality{TxID: txID}))
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

func InitApprover(ii *integration.Infrastructure, approver string) {
	_, err := ii.Client(approver).CallView("init", nil)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

func CheckLocalMetrics(ii *integration.Infrastructure, user, viewName string) {
	metrics, err := ii.WebClient(user).Metrics()
	gomega.Expect(err).To(gomega.BeNil())
	gomega.Expect(metrics).NotTo(gomega.BeEmpty())

	var sum float64
	for _, m := range metrics[ViewCallsOperationsMetric].GetMetric() {
		for _, labelPair := range m.Label {
			if labelPair.GetName() == "view" && labelPair.GetValue() == viewName {
				sum += m.Counter.GetValue()
			}
		}
	}

	logger.Infof("Received in total %f view operations for [%s] for user %s: %v", sum, viewName, user, metrics[ViewCallsOperationsMetric].GetMetric())
	gomega.Expect(sum).NotTo(gomega.BeZero(), fmt.Sprintf("Operations found: %v", metrics))
}

func CheckJaegerTraces(ii *integration.Infrastructure, nodeName, viewName string, spanMatcher gomega.OmegaMatcher) {
	cli, err := ii.NWO.JaegerReporter()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	it, err := cli.FindTraces(nodeName, viewName)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	// FlattenValues cannot tell a chunk with no spans from the end of the stream,
	// and the stream may send one, so drop the empty chunks first: otherwise the
	// spans behind them never reach the matcher below.
	nonEmpty := iterators.Filter(it, func(c *api_v2.SpansResponseChunk) bool { return len(c.Spans) > 0 })
	spans, err := iterators.ReadAllPointers(iterators.FlattenValues(nonEmpty, func(c *api_v2.SpansResponseChunk) ([]model.Span, error) { return c.Spans, nil }))

	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	logger.Infof("Received jaeger %d spans for [%s:%s]: %s", len(spans), nodeName, viewName, spans)

	if len(spans) > 0 {
		gomega.Expect(spans).To(spanMatcher)
		return
	}

	services, err := cli.GetServices()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	operations, err := cli.GetOperations("")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	logger.Infof("No spans found. %d operations found in %d services: [%v] [%v]", len(operations), len(services), services, operations)
	gomega.Expect(spans).To(spanMatcher)
}

// CheckPrometheusMetrics asserts that Prometheus has scraped at least one call of
// viewName. It polls rather than sampling once: Prometheus scrapes the FSC nodes
// every 15s (integration/nwo/monitoring/monitoring/monitoring.go), and the IOU flow
// finishes well inside a single scrape interval, so a single read races the first
// scrape and sees zero even though the node is exporting the metric.
func CheckPrometheusMetrics(ii *integration.Infrastructure, viewName string) {
	cli, err := ii.NWO.PrometheusReporter()
	gomega.Expect(err).To(gomega.BeNil())
	gomega.Eventually(func() (int, error) {
		return cli.GetViewOperations("", viewName)
	}, prometheusScrapeEventually, 2*time.Second).ShouldNot(gomega.BeZero())
}

// Tables returns the names of the tables in the Postgres database that the persistence name of
// the FSC node replica uses.
func Tables(ii *integration.Infrastructure, replica string, name driver.PersistenceName) []string {
	db := openPostgres(ii, replica, name)
	defer func() { _ = db.Close() }()

	rows, err := db.Query("SELECT table_name FROM information_schema.tables WHERE table_schema = 'public'")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	defer func() { _ = rows.Close() }()
	var tables []string
	for rows.Next() {
		var table string
		gomega.Expect(rows.Scan(&table)).To(gomega.Succeed())
		tables = append(tables, table)
	}
	gomega.Expect(rows.Err()).NotTo(gomega.HaveOccurred())
	return tables
}

// Connections returns how many connections other than its own the Postgres database of the
// persistence name of the FSC node replica holds, and how many of them use TLS.
func Connections(ii *integration.Infrastructure, replica string, name driver.PersistenceName) (total, tls int) {
	db := openPostgres(ii, replica, name)
	defer func() { _ = db.Close() }()

	err := db.QueryRow(`SELECT count(*), count(*) FILTER (WHERE s.ssl)
		FROM pg_stat_ssl s JOIN pg_stat_activity a USING (pid)
		WHERE a.datname = current_database() AND a.pid <> pg_backend_pid()`).Scan(&total, &tls)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return total, tls
}

// openPostgres connects to the Postgres database of the persistence name of the FSC node
// replica, with that persistence's TLS settings.
func openPostgres(ii *integration.Infrastructure, replica string, name driver.PersistenceName) *sql.DB {
	p, ok := ii.NWOCtx.PlatformByName(fsc.TopologyName).(*fsc.Platform)
	gomega.Expect(ok).To(gomega.BeTrue(), "expected an fsc platform")
	var opts *node.SQLOpts
	for _, r := range p.Peers {
		if r.UniqueName == replica {
			opts = r.Options.GetPostgresPersistence(name)
			break
		}
	}
	gomega.Expect(opts).NotTo(gomega.BeNil(), "replica [%s] has no postgres persistence [%s]", replica, name)

	dataSource := opts.DataSource
	if opts.TLS {
		var err error
		dataSource, err = postgres.RegisterTLSConnection(dataSource, postgres.TLSConfig{SSLMode: "verify-full", RootCertPath: opts.TLSRootCertPath})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}
	db, err := sql.Open("pgx", dataSource)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return db
}
