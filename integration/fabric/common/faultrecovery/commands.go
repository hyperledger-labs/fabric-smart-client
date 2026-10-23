/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package faultrecovery

import (
	"context"
	"encoding/json"
	"time"

	"github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery/views"
	cviews "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
)

const (
	// viewTimeout bounds each view call, so a node that never answers fails the spec
	// instead of hanging it.
	viewTimeout = 2 * time.Minute
	// pollTimeout bounds one polled view call, so a call that blocks while a node
	// restarts is retried instead of using up the caller's whole timeout.
	pollTimeout = 20 * time.Second
)

// SubmitAndWaitFinal endorses and orders n transactions between alice and bob, one
// after the other, and returns their IDs once each is final on both nodes.
//
// Finality is polled, so a node that is still restarting is retried until viewTimeout.
func SubmitAndWaitFinal(ii *integration.Infrastructure, n int) []string {
	txIDs := make([]string, 0, n)
	for i := range n {
		txIDs = append(txIDs, Order(ii, Endorse(ii, i)))
	}

	for _, node := range []string{"alice", "bob"} {
		for _, txID := range txIDs {
			gomega.EventuallyWithOffset(1, func() error {
				ctx, cancel := context.WithTimeout(context.Background(), pollTimeout)
				defer cancel()
				_, err := ii.Client(node).CallViewWithContext(ctx, "finality", common.JSONMarshall(&cviews.Finality{TxID: txID}))
				return err
			}, viewTimeout, time.Second).Should(gomega.Succeed(), "tx [%s] not final on [%s]", txID, node)
		}
	}
	return txIDs
}

// Endorse runs alice's endorse view with bob as responder and returns the endorsed
// transaction, not yet ordered, for Order. On Fabric, bob's vault then holds the
// transaction as Busy.
func Endorse(ii *integration.Infrastructure, value int) []byte {
	// A view's []byte result reaches the client as is, not JSON-encoded.
	return callView(ii, "alice", "endorse", &views.Submit{Value: value, Responder: ii.Identity("bob")}).([]byte)
}

// Order runs alice's order view on a transaction Endorse returned and returns its ID
// once it is final on alice.
func Order(ii *integration.Infrastructure, tx []byte) string {
	return common.JSONUnmarshalString(callView(ii, "alice", "order", &views.Order{Tx: tx}))
}

// Query returns what node knows about the transaction txID; see views.QueryView.
func Query(ii *integration.Infrastructure, node, txID string) (*views.QueryResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), pollTimeout)
	defer cancel()
	raw, err := ii.Client(node).CallViewWithContext(ctx, "query", common.JSONMarshall(&views.Query{TxID: txID}))
	if err != nil {
		return nil, err
	}
	res := &views.QueryResult{}
	if err := json.Unmarshal(raw.([]byte), res); err != nil {
		return nil, err
	}
	return res, nil
}

func callView(ii *integration.Infrastructure, node, fid string, in any) any {
	ctx, cancel := context.WithTimeout(context.Background(), viewTimeout)
	defer cancel()
	res, err := ii.Client(node).CallViewWithContext(ctx, fid, common.JSONMarshall(in))
	gomega.ExpectWithOffset(2, err).NotTo(gomega.HaveOccurred(), "view [%s] on [%s] failed", fid, node)
	return res
}
