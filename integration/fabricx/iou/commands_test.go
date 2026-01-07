/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iou_test

import (
	"context"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	views2 "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	. "github.com/onsi/gomega"
)

const timeout = 2 * time.Minute

func CreateIOU(ii *integration.Infrastructure, identityLabel string, amount uint, approver string) (string, error) {
	return CreateIOUWithBorrower(ii, "borrower", identityLabel, amount, approver)
}

func CreateIOUWithBorrower(ii *integration.Infrastructure, borrower, identityLabel string, amount uint, approver string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	res, err := ii.Client(borrower).CallViewWithContext(ctx,
		"create", common.JSONMarshall(&views.Create{
			Amount:   amount,
			Identity: identityLabel,
			Lender:   ii.Identity("lender"),
			Approver: ii.Identity(approver),
		}),
	)
	if err != nil {
		return "", err
	}

	Expect(res).NotTo(BeNil())
	return common.JSONUnmarshalString(res), nil
}

func CheckState(ii *integration.Infrastructure, partyID, iouStateID string, expected int) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	res, err := ii.CLI(partyID).CallViewWithContext(ctx, "query", common.JSONMarshall(&views.Query{LinearID: iouStateID}))
	Expect(err).NotTo(HaveOccurred())
	Expect(common.JSONUnmarshalInt(res)).To(BeEquivalentTo(expected))
}

func UpdateIOU(ii *integration.Infrastructure, iouStateID string, amount uint, approver string, expectedErrs ...string) {
	UpdateIOUWithBorrower(ii, "borrower", iouStateID, amount, approver, expectedErrs...)
}

func UpdateIOUWithBorrower(ii *integration.Infrastructure, borrower, iouStateID string, amount uint, approver string, expectedErrs ...string) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	txIDBoxed, err := ii.Client(borrower).CallViewWithContext(ctx, "update",
		common.JSONMarshall(&views.Update{
			LinearID: iouStateID,
			Amount:   amount,
			Approver: ii.Identity(approver),
		}),
	)
	if len(expectedErrs) > 0 {
		// check that we got the expected errors (if defined) from the update view
		errStr := err.Error()
		Expect(err).To(HaveOccurred())
		for _, s := range expectedErrs {
			Expect(errStr).To(ContainSubstring(s))
		}
		return
	}

	Expect(err).NotTo(HaveOccurred())

	// next check lender has observed the tx
	txID := common.JSONUnmarshalString(txIDBoxed)
	CheckFinalityWithLender(ii, "lender", txID)
}

func CheckFinalityWithLender(ii *integration.Infrastructure, lender string, txID string) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	_, err := ii.Client(lender).CallViewWithContext(ctx, "finality", common.JSONMarshall(views2.Finality{TxID: txID}))
	Expect(err).NotTo(HaveOccurred())
}

func InitApprover(ii *integration.Infrastructure, approver string) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	_, err := ii.Client(approver).CallViewWithContext(ctx, "init", nil)
	Expect(err).NotTo(HaveOccurred())
}
