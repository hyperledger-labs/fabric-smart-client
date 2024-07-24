/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iou

import (
	"context"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	. "github.com/onsi/gomega"
	"github.com/prometheus/common/model"
)

var logger = flogging.MustGetLogger("iou-test")

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
	Expect(err).NotTo(HaveOccurred())
	Expect(res).NotTo(BeNil())
	return common.JSONUnmarshalString(res)
}

func CheckState(ii *integration.Infrastructure, partyID, iouStateID string, expected int) {
	res, err := ii.CLI(partyID).CallView("query", common.JSONMarshall(&views.Query{LinearID: iouStateID}))
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalInt(res)).To(BeEquivalentTo(expected))
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
	Expect(err).NotTo(HaveOccurred())
	txID := common.JSONUnmarshalString(txIDBoxed)
	Expect(ii.Client("lender").IsTxFinal(txID)).NotTo(HaveOccurred())
}

func InitApprover(ii *integration.Infrastructure, approver string) {
	_, err := ii.Client(approver).CallView("init", nil)
	Expect(err).NotTo(HaveOccurred())
}

func CheckLocalMetrics(ii *integration.Infrastructure, user string, viewName string) {
	metrics, err := ii.WebClient(user).Metrics()
	Expect(err).To(BeNil())
	Expect(metrics).NotTo(BeEmpty())

	var sum float64
	for _, m := range metrics[user+"_fsc_view_operations"].GetMetric() {
		for _, labelPair := range m.Label {
			if labelPair.GetName() == "view" && labelPair.GetValue() == viewName {
				sum += m.Counter.GetValue()
			}
		}
	}

	logger.Infof("Received in total %f view operations for [%s] for user %s: %v", sum, viewName, user, metrics["fsc_view_operations"].GetMetric())
	Expect(sum).NotTo(BeZero())
}

func CheckPrometheusMetrics(ii *integration.Infrastructure, viewName string) {
	cli, err := ii.NWO.PrometheusAPI()
	Expect(err).To(BeNil())
	metric := model.Metric{
		"__name__": "fsc_view_operations",
		"view":     model.LabelValue(viewName),
	}
	val, warnings, err := cli.Query(context.Background(), metric.String(), time.Now())
	Expect(warnings).To(BeEmpty())
	Expect(err).To(BeNil())
	Expect(val.Type()).To(Equal(model.ValVector))

	logger.Infof("Received prometheus metrics for view [%s]: %s", viewName, val)

	vector, ok := val.(model.Vector)
	Expect(ok).To(BeTrue())
	Expect(vector).To(HaveLen(1))
	Expect(vector[0].Value).NotTo(Equal(model.SampleValue(0)))
}
