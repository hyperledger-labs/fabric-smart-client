/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package twonets

import (
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/twonets/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

// Endorsement is a transaction Initiator builds on Network, writing to Namespace,
// and commits with its own endorsement and that of Endorser.
type Endorsement struct {
	Initiator, Endorser, Network, Namespace string
}

// EndorseAcross runs each endorsement and expects its transaction to be valid in
// the vault of its network on both nodes.
func EndorseAcross(ii *integration.Infrastructure, endorsements []Endorsement) {
	for _, e := range endorsements {
		ginkgo.By(e.Initiator + " builds on " + e.Network + " and collects " + e.Endorser + "'s endorsement")
		res, err := ii.Client(e.Initiator).CallView("endorse", common.JSONMarshall(&views.Endorse{
			Network: e.Network, Namespace: e.Namespace, Endorser: e.Endorser,
		}))
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		expectValid(ii, e, common.JSONUnmarshalString(res))
	}
}

// EndorseAcrossEventually is EndorseAcross that retries each endorsement until it succeeds
// or timeout passes, for a network whose peers are coming back: until they serve again, the
// initiator cannot read the channel it binds the transaction to, and the endorser refuses it.
func EndorseAcrossEventually(ii *integration.Infrastructure, endorsements []Endorsement, timeout time.Duration) {
	for _, e := range endorsements {
		ginkgo.By(e.Initiator + " builds on " + e.Network + " and collects " + e.Endorser + "'s endorsement, retrying")
		var txID string
		gomega.Eventually(func() error {
			res, err := ii.Client(e.Initiator).CallView("endorse", common.JSONMarshall(&views.Endorse{
				Network: e.Network, Namespace: e.Namespace, Endorser: e.Endorser,
			}))
			if err != nil {
				return err
			}
			txID = common.JSONUnmarshalString(res)
			return nil
		}).WithTimeout(timeout).WithPolling(time.Second).Should(gomega.Succeed(),
			"%s never endorsed on [%s]", e.Initiator, e.Network)
		expectValid(ii, e, txID)
	}
}

// expectValid waits until txID is valid in the vault of e's network on both nodes of e.
func expectValid(ii *integration.Infrastructure, e Endorsement, txID string) {
	for _, node := range []string{e.Initiator, e.Endorser} {
		gomega.Eventually(func() int {
			res, err := ii.Client(node).CallView("status", common.JSONMarshall(&views.Status{Network: e.Network, TxID: txID}))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			return common.JSONUnmarshalInt(res)
		}).WithTimeout(time.Minute).WithPolling(time.Second).Should(gomega.Equal(driver.Valid),
			"status of [%s] on [%s] at %s", txID, e.Network, node)
	}
}
