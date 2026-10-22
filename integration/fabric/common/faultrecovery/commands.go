/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package faultrecovery

import (
	"context"
	"time"

	"github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery/views"
	cviews "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
)

const (
	// viewTimeout bounds each submit, so a node that never answers fails the spec
	// instead of hanging it.
	viewTimeout = 2 * time.Minute
	// finalityPollTimeout bounds one finality check, so a check that blocks while a
	// node restarts is retried within viewTimeout instead of using it up.
	finalityPollTimeout = 20 * time.Second
)

// SubmitAndWaitFinal runs alice's submit view n times, one transaction after the
// other, and returns their IDs once each is final on both alice and bob.
//
// Finality is polled, so a node that is still restarting is retried until viewTimeout.
func SubmitAndWaitFinal(ii *integration.Infrastructure, n int) []string {
	txIDs := make([]string, 0, n)
	for i := range n {
		ctx, cancel := context.WithTimeout(context.Background(), viewTimeout)
		res, err := ii.Client("alice").CallViewWithContext(ctx, "submit", common.JSONMarshall(&views.Submit{
			Value:     i,
			Responder: ii.Identity("bob"),
		}))
		cancel()
		gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred(), "submit %d of %d failed", i+1, n)
		txIDs = append(txIDs, common.JSONUnmarshalString(res))
	}

	for _, node := range []string{"alice", "bob"} {
		for _, txID := range txIDs {
			gomega.EventuallyWithOffset(1, func() error {
				ctx, cancel := context.WithTimeout(context.Background(), finalityPollTimeout)
				defer cancel()
				_, err := ii.Client(node).CallViewWithContext(ctx, "finality", common.JSONMarshall(&cviews.Finality{TxID: txID}))
				return err
			}, viewTimeout, time.Second).Should(gomega.Succeed(), "tx [%s] not final on [%s]", txID, node)
		}
	}
	return txIDs
}
