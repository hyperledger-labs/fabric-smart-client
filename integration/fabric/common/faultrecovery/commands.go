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
	dto "github.com/prometheus/client_model/go"

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
func SubmitAndWaitFinal(ii *integration.Infrastructure, n int) []string {
	txIDs := make([]string, 0, n)
	for i := range n {
		txIDs = append(txIDs, Order(ii, Endorse(ii, i)))
	}
	WaitFinal(ii, txIDs...)
	return txIDs
}

// WaitFinal returns once every transaction in txIDs is final on alice and on bob.
//
// Finality is polled, so a node that is still restarting is retried until viewTimeout.
func WaitFinal(ii *integration.Infrastructure, txIDs ...string) {
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
}

// Endorse runs alice's endorse view with bob as responder and returns the endorsed
// transaction, not yet ordered, for Order. On Fabric, bob's vault then holds the
// transaction as Busy.
func Endorse(ii *integration.Infrastructure, value int) *views.Endorsed {
	tx := &views.Endorsed{}
	common.JSONUnmarshal(callView(ii, "alice", "endorse", &views.Submit{Value: value, Responder: ii.Identity("bob")}).([]byte), tx)
	return tx
}

// Order runs alice's order view on a transaction Endorse returned and returns its ID
// once it is final on alice.
func Order(ii *integration.Infrastructure, tx *views.Endorsed) string {
	return common.JSONUnmarshalString(callView(ii, "alice", "order", &views.Order{Tx: tx.Tx}))
}

// OrderEventually is Order for a network that rejects broadcasts while it recovers,
// such as raft orderers electing a new leader: it retries the order view until it
// succeeds. An attempt can take as long as the view's own finality timeout, so the
// retries get several attempts' worth of time.
func OrderEventually(ii *integration.Infrastructure, tx *views.Endorsed) {
	gomega.EventuallyWithOffset(1, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), viewTimeout)
		defer cancel()
		_, err := ii.Client("alice").CallViewWithContext(ctx, "order", common.JSONMarshall(&views.Order{Tx: tx.Tx}))
		return err
	}, 3*viewTimeout, time.Second).Should(gomega.Succeed(), "tx [%s] not ordered", tx.TxID)
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

// MetricSum returns the sum of every series of the counter or gauge family name on
// node's metrics endpoint; zero if the node exports none, as it does for a counter
// never incremented.
func MetricSum(ii *integration.Infrastructure, node, name string) float64 {
	families, err := ii.WebClient(node).Metrics()
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred())
	return sumFamily(families, name)
}

// sumFamily returns the sum of every series of the counter or gauge family name in
// families; zero if families has none.
func sumFamily(families map[string]*dto.MetricFamily, name string) float64 {
	var sum float64
	for _, m := range families[name].GetMetric() {
		// A family holds one metric type; the getter of the other returns zero.
		sum += m.GetCounter().GetValue() + m.GetGauge().GetValue()
	}
	return sum
}

func callView(ii *integration.Infrastructure, node, fid string, in any) any {
	ctx, cancel := context.WithTimeout(context.Background(), viewTimeout)
	defer cancel()
	res, err := ii.Client(node).CallViewWithContext(ctx, fid, common.JSONMarshall(in))
	gomega.ExpectWithOffset(2, err).NotTo(gomega.HaveOccurred(), "view [%s] on [%s] failed", fid, node)
	return res
}

// Echo runs alice's echo view against bob; with block, bob's responder holds the session
// open without replying. It returns the view's error.
func Echo(ii *integration.Infrastructure, block bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), viewTimeout)
	defer cancel()
	_, err := ii.Client("alice").CallViewWithContext(ctx, "echo", common.JSONMarshall(&views.Echo{Responder: ii.Identity("bob"), Block: block}))
	return err
}
