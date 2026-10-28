/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

import (
	"os"
	"path/filepath"
	"time"

	"github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/client"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
)

// raftIsLeaderMetric is the gauge an etcdraft orderer sets to 1 for every
// channel it leads.
const raftIsLeaderMetric = "consensus_etcdraft_is_leader"

// WaitForRaftLeader blocks until one of the orderers reports itself as the
// raft leader of channel, bounded by n.EventuallyTimeout. A channel accepts
// transactions only once it has a leader; the channel participation API
// reports a joined channel as active before the election completes, so its
// status is no substitute. The leader is read from the orderers' operations
// endpoint; an orderer whose endpoint fails does not hide a leader reported by
// another. It returns at once for networks that do not run etcdraft: a BFT
// orderer reports its configured leader as soon as the chain starts, so its
// leader metric says nothing about readiness.
func (n *Network) WaitForRaftLeader(channel string, orderers ...*topology.Orderer) {
	if n.Consensus == nil || n.Consensus.Type != "etcdraft" {
		return
	}
	clients := make([]*client.MetricsClient, len(orderers))
	for i, o := range orderers {
		c, err := client.NewMetricsClient(n.ordererOperationsConfig(o))
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		clients[i] = c
	}
	isLeader := func() (bool, error) {
		var lastErr error
		for _, c := range clients {
			families, err := c.Metrics()
			if err != nil {
				lastErr = err
				continue
			}
			for _, m := range families[raftIsLeaderMetric].GetMetric() {
				for _, l := range m.GetLabel() {
					if l.GetName() == "channel" && l.GetValue() == channel && m.GetGauge().GetValue() == 1 {
						return true, nil
					}
				}
			}
		}
		return false, lastErr
	}
	gomega.Eventually(isLeader, n.EventuallyTimeout, 100*time.Millisecond).Should(
		gomega.BeTrue(), "no raft leader for channel [%s]", channel)
}

// ordererOperationsConfig locates o's operations endpoint. With TLS enabled,
// the endpoint serves /metrics only to clients that present a certificate
// issued by a CA it trusts, whether or not its listener requires client
// authentication, so the orderer's own TLS material serves as trust anchor and
// as client certificate.
func (n *Network) ordererOperationsConfig(o *topology.Orderer) *grpc.ConnectionConfig {
	cc := &grpc.ConnectionConfig{Address: n.OrdererAddress(o, OperationsPort), ConnectionTimeout: time.Second}
	if !n.TLSEnabled {
		return cc
	}
	read := func(name string) []byte {
		raw, err := os.ReadFile(filepath.Join(n.OrdererLocalTLSDir(o), name))
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		return raw
	}
	cc.TLS = grpc.SecureOptions{
		UseTLS:            true,
		ServerRootCAs:     [][]byte{read("ca.crt")},
		Certificate:       read("server.crt"),
		Key:               read("server.key"),
		RequireClientCert: true,
	}
	return cc
}
