/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iou

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	cviews "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
	model2 "github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
	"github.com/onsi/gomega"
)

const (
	ViewCallsOperationsMetric = "fsc_view_services_view_calls_operations"
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

func CheckLocalMetrics(ii *integration.Infrastructure, user string, viewName string) {
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

	spans, err := iterators.ReadAllPointers(iterators.FlattenValues(it, func(c *api_v2.SpansResponseChunk) ([]model2.Span, error) { return c.Spans, nil }))

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

func CheckPrometheusMetrics(ii *integration.Infrastructure, viewName string) {
	cli, err := ii.NWO.PrometheusReporter()
	gomega.Expect(err).To(gomega.BeNil())
	ops, err := cli.GetViewOperations("", viewName)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	gomega.Expect(ops).ToNot(gomega.BeZero())
}
