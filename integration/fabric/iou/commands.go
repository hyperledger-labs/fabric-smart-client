/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iou

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	. "github.com/onsi/gomega"
)

func CreateIOU(ii *integration.Infrastructure, identityLabel string, amount uint, approver string) string {
	res, err := ii.Client("borrower").CallView(
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
	txIDBoxed, err := ii.Client("borrower").CallView("update",
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
