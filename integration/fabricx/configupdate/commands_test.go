/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configupdate_test

import (
	"context"
	"time"

	"github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
)

// defaultViewTimeout bounds every view call these specs make.
//
// The shared integration/fabric/iou helpers call CallView with no context, so a view that
// never answers blocks the spec for as long as Ginkgo lets it -- two runs during
// development hung for over twenty minutes. Bounding each call turns that into a failure
// that names the call which hung and arrives in two minutes, which is why the sibling
// fabricx suites define their own helpers rather than reusing the shared ones (see
// integration/fabricx/iou/commands_test.go). Do not replace these with the
// integration/fabric/iou equivalents.
const defaultViewTimeout = 2 * time.Minute

// InitApprover runs the named approver's init view, which is what registers the iou
// namespace's endorsement policy with that node.
func InitApprover(ii *integration.Infrastructure, approver string) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultViewTimeout)
	defer cancel()

	_, err := ii.Client(approver).CallViewWithContext(ctx, "init", nil)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "the [%s] init view failed", approver)
}

// CreateIOU has the borrower create an IOU for the given amount, endorsed by the named
// approver, and returns the linear ID of the state it wrote.
func CreateIOU(ii *integration.Infrastructure, amount uint, approver string) string {
	ctx, cancel := context.WithTimeout(context.Background(), defaultViewTimeout)
	defer cancel()

	res, err := ii.Client("borrower").CallViewWithContext(ctx, "create",
		common.JSONMarshall(&views.Create{
			Amount:   amount,
			Lender:   ii.Identity("lender"),
			Approver: ii.Identity(approver),
		}),
	)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "the create view failed for amount [%d]", amount)
	gomega.Expect(res).NotTo(gomega.BeNil())

	return common.JSONUnmarshalString(res)
}

// CheckState asserts that the named party's own query view reports the given IOU at the
// expected amount.
//
// It queries the node's view service rather than its command line, which is what
// integration/fabric/iou does: the CLI client ignores the context it is handed and bounds
// itself at ten minutes instead, and nothing in these specs is about the CLI.
func CheckState(ii *integration.Infrastructure, party, iouStateID string, expected int) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultViewTimeout)
	defer cancel()

	res, err := ii.Client(party).CallViewWithContext(ctx, "query",
		common.JSONMarshall(&views.Query{LinearID: iouStateID}))
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "the [%s] query view failed", party)
	gomega.Expect(common.JSONUnmarshalInt(res)).To(gomega.BeEquivalentTo(expected))
}
