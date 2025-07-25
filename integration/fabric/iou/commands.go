/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iou

import (
	"context"
	"fmt"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	cviews "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	model2 "github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
	"github.com/onsi/gomega"
	"github.com/prometheus/common/model"
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
	for _, m := range metrics["fsc_view_operations"].GetMetric() {
		for _, labelPair := range m.Label {
			if labelPair.GetName() == "view" && labelPair.GetValue() == viewName {
				sum += m.Counter.GetValue()
			}
		}
	}

	logger.Infof("Received in total %f view operations for [%s] for user %s: %v", sum, viewName, user, metrics["fsc_view_operations"].GetMetric())
	gomega.Expect(sum).NotTo(gomega.BeZero(), fmt.Sprintf("Operations found: %v", metrics))
}

func CheckJaegerTraces(ii *integration.Infrastructure, nodeName, viewName string, spanMatcher gomega.OmegaMatcher) {
	cli, err := ii.NWO.JaegerAPI()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	findTraces, err := cli.FindTraces(context.Background(), &api_v2.FindTracesRequest{Query: &api_v2.TraceQueryParameters{ServiceName: nodeName, OperationName: viewName}})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	spans := make([]model2.Span, 0)
	for chunk, err := findTraces.Recv(); chunk != nil; chunk, err = findTraces.Recv() {
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		spans = append(spans, chunk.Spans...)
	}
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	logger.Infof("Received jaeger %d spans for [%s:%s]: %s", len(spans), nodeName, viewName, spans)

	if len(spans) > 0 {
		gomega.Expect(spans).To(spanMatcher)
		return
	}

	services, err := cli.GetServices(context.Background(), &api_v2.GetServicesRequest{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	operations, err := cli.GetOperations(context.Background(), &api_v2.GetOperationsRequest{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	logger.Infof("No spans found. %d operations found in %d services: [%v] [%v]", len(operations.GetOperations()), len(services.GetServices()), services.GetServices(), operations.GetOperationNames())
	gomega.Expect(spans).To(spanMatcher)
}

func CheckPrometheusMetrics(ii *integration.Infrastructure, viewName string) {
	cli, err := ii.NWO.PrometheusAPI()
	gomega.Expect(err).To(gomega.BeNil())
	metric := model.Metric{
		"__name__": model.LabelValue("fsc_view_operations"),
		"view":     model.LabelValue(viewName),
	}
	val, warnings, err := cli.Query(context.Background(), metric.String(), time.Now())
	gomega.Expect(warnings).To(gomega.BeEmpty())
	gomega.Expect(err).To(gomega.BeNil())
	gomega.Expect(val.Type()).To(gomega.Equal(model.ValVector))

	logger.Infof("Received prometheus metrics for view [%s]: %s", viewName, val)

	vector, ok := val.(model.Vector)
	gomega.Expect(ok).To(gomega.BeTrue())
	gomega.Expect(vector).To(gomega.HaveLen(1))
	gomega.Expect(vector[0].Value).NotTo(gomega.Equal(model.SampleValue(0)))
}
