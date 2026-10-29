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
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	nwonode "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
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
	return SubmitAndWaitFinalFrom(ii, "alice", n)
}

// SubmitAndWaitFinalFrom is SubmitAndWaitFinal with initiator, alice or one of her replica
// IDs such as fsc.alice.0, in alice's place.
func SubmitAndWaitFinalFrom(ii *integration.Infrastructure, initiator string, n int) []string {
	txIDs := make([]string, 0, n)
	for i := range n {
		tx := endorseFrom(ii, initiator, i)
		txIDs = append(txIDs, orderFrom(ii, initiator, tx))
	}
	WaitFinalOn(ii, initiator, txIDs...)
	WaitFinalOn(ii, "bob", txIDs...)
	return txIDs
}

// WaitFinal returns once every transaction in txIDs is final on alice and on bob.
//
// Finality is polled, so a node that is still restarting is retried until viewTimeout.
func WaitFinal(ii *integration.Infrastructure, txIDs ...string) {
	WaitFinalOn(ii, "alice", txIDs...)
	WaitFinalOn(ii, "bob", txIDs...)
}

// WaitFinalOn waits until each of txIDs is final on node, a node name or a replica ID such as fsc.bob.1.
func WaitFinalOn(ii *integration.Infrastructure, node string, txIDs ...string) {
	for _, txID := range txIDs {
		gomega.EventuallyWithOffset(1, func() error {
			ctx, cancel := context.WithTimeout(context.Background(), pollTimeout)
			defer cancel()
			_, err := ii.Client(node).CallViewWithContext(ctx, "finality", common.JSONMarshall(&cviews.Finality{TxID: txID}))
			return err
		}, viewTimeout, time.Second).Should(gomega.Succeed(), "tx [%s] not final on [%s]", txID, node)
	}
}

// Endorse runs alice's endorse view with bob as responder and returns the endorsed
// transaction, not yet ordered, for Order. On Fabric, bob's vault then holds the
// transaction as Busy.
func Endorse(ii *integration.Infrastructure, value int) *views.Endorsed {
	return endorseFrom(ii, "alice", value)
}

func endorseFrom(ii *integration.Infrastructure, initiator string, value int) *views.Endorsed {
	tx := &views.Endorsed{}
	common.JSONUnmarshal(callView(ii, initiator, "endorse", &views.Submit{Value: value, Responder: ii.Identity("bob")}).([]byte), tx)
	return tx
}

// Order runs alice's order view on a transaction Endorse returned and returns its ID
// once it is final on alice.
func Order(ii *integration.Infrastructure, tx *views.Endorsed) string {
	return orderFrom(ii, "alice", tx)
}

func orderFrom(ii *integration.Infrastructure, initiator string, tx *views.Endorsed) string {
	return common.JSONUnmarshalString(callView(ii, initiator, "order", &views.Order{Tx: tx.Tx}))
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

// LedgerOf returns what node reads through the ledger API about txID, a committed
// transaction, and the error it gets for unknownTxID; see cviews.LedgerView.
func LedgerOf(ii *integration.Infrastructure, node, txID, unknownTxID string) *cviews.LedgerResult {
	res := &cviews.LedgerResult{}
	common.JSONUnmarshal(callView(ii, node, "ledger", &cviews.Ledger{TxID: txID, UnknownTxID: unknownTxID}).([]byte), res)
	return res
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
	return EchoWith(ii, ii.Client("alice"), block)
}

// EchoWith is Echo called through c, a view client of alice.
func EchoWith(ii *integration.Infrastructure, c api.ViewClient, block bool) error {
	return callEcho(c, &views.Echo{Responder: ii.Identity("bob"), Block: block})
}

// AwaitEcho runs alice's echo view through c, a view client of alice, with bob's responder
// waiting for a second message that never comes. It returns the view's error.
func AwaitEcho(ii *integration.Infrastructure, c api.ViewClient) error {
	return callEcho(c, &views.Echo{Responder: ii.Identity("bob"), Await: true})
}

func callEcho(c api.ViewClient, in *views.Echo) error {
	ctx, cancel := context.WithTimeout(context.Background(), viewTimeout)
	defer cancel()
	_, err := c.CallViewWithContext(ctx, "echo", common.JSONMarshall(in))
	return err
}

// ConnectedReplica returns the replica of node that holds the one connection open through
// node's load balancer, and node's other replicas.
func ConnectedReplica(ii *integration.Infrastructure, node string) (*nwonode.Replica, []*nwonode.Replica) {
	conns := ii.FscPlatform.LoadBalancer(node).Connections()
	gomega.ExpectWithOffset(1, conns).To(gomega.HaveLen(1), "connections through the load balancer of [%s]", node)
	var held *nwonode.Replica
	var others []*nwonode.Replica
	for _, r := range ii.FscPlatform.Peers {
		if r.Name != node {
			continue
		}
		if _, ok := conns[ii.FscPlatform.PeerAddress(r, fsc.P2PPort)]; ok {
			held = r
		} else {
			others = append(others, r)
		}
	}
	gomega.ExpectWithOffset(1, held).NotTo(gomega.BeNil(), "no replica of [%s] holds connection %v", node, conns)
	return held, others
}
