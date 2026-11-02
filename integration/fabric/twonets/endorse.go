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
		txID := common.JSONUnmarshalString(res)

		for _, node := range []string{e.Initiator, e.Endorser} {
			gomega.Eventually(func() int {
				res, err := ii.Client(node).CallView("status", common.JSONMarshall(&views.Status{Network: e.Network, TxID: txID}))
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return common.JSONUnmarshalInt(res)
			}).WithTimeout(time.Minute).WithPolling(time.Second).Should(gomega.Equal(driver.Valid),
				"status of [%s] on [%s] at %s", txID, e.Network, node)
		}
	}
}
