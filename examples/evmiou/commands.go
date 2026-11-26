/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package evmiou

import (
	"github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/iou/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
)

// CreateIOU has the borrower create an IOU of amount owed to the lender, approved by approver,
// and returns its id.
func CreateIOU(ii *integration.Infrastructure, amount uint64, approver string) string {
	res, err := ii.Client("borrower").CallView("create", common.JSONMarshall(&views.CreateIOUView{
		Amount:   amount,
		Lender:   "lender",
		Approver: approver,
	}))
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	return common.JSONUnmarshalString(res)
}

// UpdateIOU has the borrower lower the IOU with id to amount, endorsed by the lender and approved
// by approver.
func UpdateIOU(ii *integration.Infrastructure, id string, amount uint64, approver string) {
	_, err := ii.Client("borrower").CallView("update", common.JSONMarshall(&views.UpdateIOUView{
		ID:       id,
		Amount:   amount,
		Lender:   "lender",
		Approver: approver,
	}))
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

// CheckState checks that party reads amount expected for the IOU with id.
func CheckState(ii *integration.Infrastructure, party, id string, expected uint64) {
	res, err := ii.Client(party).CallView("query", common.JSONMarshall(&views.QueryView{ID: id}))
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(common.JSONUnmarshalInt(res)).To(gomega.BeEquivalentTo(expected))
}
