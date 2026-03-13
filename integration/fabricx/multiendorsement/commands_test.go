/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package multiendorsement_test

import (
	"context"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/simple/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"

	. "github.com/onsi/gomega"
)

const (
	// defaultViewTimeout is the default timeout for view calls
	defaultViewTimeout = 2 * time.Minute
)

func CallCreateWithApprovers(ii *integration.Infrastructure, identityLabel string, amount uint, approvers ...string) (string, error) {
	return CreateMultiendorsementWithCreatorAndApprovers(ii, "creator", identityLabel, amount, approvers...)
}

func CreateMultiendorsementWithCreatorAndApprovers(ii *integration.Infrastructure, creator, identityLabel string, amount uint, approvers ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultViewTimeout)
	defer cancel()

	ids := make([]view.Identity, 0, len(approvers))
	for _, approver := range approvers {
		ids = append(ids, ii.Identity(approver))
	}

	res, err := ii.Client(creator).CallViewWithContext(
		ctx,
		"create", common.JSONMarshall(&views.CreateParams{
			Owner:     identityLabel,
			Value:     int(amount),
			Namespace: "simple",
			Approvers: ids,
		}),
	)
	if err != nil {
		return "", err
	}

	Expect(res).NotTo(BeNil())
	return common.JSONUnmarshalString(res), nil
}

func CheckState(ii *integration.Infrastructure, partyID string, testObjects []views.SomeObject) {
	res, err := ii.CLI(partyID).CallView("query", common.JSONMarshall(&views.QueryParams{
		SomeIDs:   []string{testObjects[0].Owner},
		Namespace: "simple",
	}))
	Expect(err).ToNot(HaveOccurred())

	raw, ok := res.(string)
	Expect(ok).To(BeTrue(), "expected string response from query view")

	var objs []views.SomeObject
	common.JSONUnmarshal([]byte(raw), &objs)

	Expect(objs).To(ConsistOf(testObjects))
}
