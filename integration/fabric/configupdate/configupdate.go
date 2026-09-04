/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package configupdate drives Fabric channel configuration changes against a running
// network, so that a test can observe how FSC nodes react to a CONFIG block that arrives
// while they are up.
//
// The FSC-side path under test lives in platform/fabric/core/generic/committer: a CONFIG
// block reaches Committer.HandleConfig, which commits it to the vault, hands the envelope
// to MembershipService.Update, and then calls applyConfigUpdates to re-read the channel's
// orderer configuration and reconfigure the ordering service. Until this suite existed
// that path only ever ran for a channel's genesis block, during start-up catch-up, and
// nothing asserted on it.
package configupdate

import (
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	protosorderer "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	nwocommon "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/network"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil"
)

// BatchTimeoutKey is the key of the BatchTimeout value in the channel config's Orderer group.
const BatchTimeoutKey = "BatchTimeout"

// OrdererGroupKey is the key of the Orderer group in the channel config.
const OrdererGroupKey = "Orderer"

// channelHandles are the nwo handles needed to submit a configuration update: the network,
// the channel to update, an orderer to submit through, and a Fabric peer whose local
// configuration the peer CLI is invoked with.
type channelHandles struct {
	Network *network.Network
	Channel string
	Orderer *topology.Orderer
	Peer    *topology.Peer
}

// handles locates the single Fabric platform in the running infrastructure and derives the
// channel, orderer and submitting peer from its topology.
func handles(ii *integration.Infrastructure) *channelHandles {
	platforms := ii.NWOCtx.PlatformsByType("fabric")
	gomega.Expect(platforms).To(gomega.HaveLen(1), "expected exactly one fabric platform")

	p, ok := platforms[0].(*fabric.Platform)
	gomega.Expect(ok).To(gomega.BeTrue(), "expected the fabric platform to be a *fabric.Platform")

	n := p.Network
	gomega.Expect(n.Channels).NotTo(gomega.BeEmpty(), "the fabric topology declares no channel")
	gomega.Expect(n.Orderers).NotTo(gomega.BeEmpty(), "the fabric topology declares no orderer")

	channel := n.Channels[0].Name
	// PeersWithChannel returns only peers of type FabricPeer, never FSC nodes, and sorts
	// them deterministically.
	peers := n.PeersWithChannel(channel)
	gomega.Expect(peers).NotTo(gomega.BeEmpty(), "no fabric peer has joined channel [%s]", channel)

	return &channelHandles{
		Network: n,
		Channel: channel,
		Orderer: n.Orderers[0],
		Peer:    peers[0],
	}
}

// BatchTimeout returns the channel's current orderer BatchTimeout.
func BatchTimeout(ii *integration.Infrastructure) time.Duration {
	h := handles(ii)
	config := network.GetConfig(h.Network, h.Peer, h.Orderer, h.Channel)
	return BatchTimeoutOf(config)
}

// SetBatchTimeout computes, signs and submits an orderer-signed channel configuration
// update that sets the orderer's BatchTimeout to the given value, and returns once the
// resulting configuration block has been committed.
//
// BatchTimeout is chosen deliberately: it lives in the Orderer group, which is exactly
// what committer.applyConfigUpdates re-reads through MembershipService.OrdererConfig
// before reconfiguring the ordering service, and changing it cannot partition the network
// or invalidate any identity, so a failure of the following flow points at FSC rather
// than at the fixture.
func SetBatchTimeout(ii *integration.Infrastructure, timeout time.Duration) {
	h := handles(ii)

	config := network.GetConfig(h.Network, h.Peer, h.Orderer, h.Channel)
	gomega.Expect(BatchTimeoutOf(config)).NotTo(gomega.Equal(timeout),
		"BatchTimeout is already [%s]; an update that changes nothing cannot be computed", timeout)

	updated, ok := proto.Clone(config).(*common.Config)
	gomega.Expect(ok).To(gomega.BeTrue(), "expected the cloned config to be a *common.Config")

	updated.ChannelGroup.Groups[OrdererGroupKey].Values[BatchTimeoutKey] = &common.ConfigValue{
		ModPolicy: "Admins",
		Value:     protoutil.MarshalOrPanic(&protosorderer.BatchTimeout{Timeout: timeout.String()}),
	}

	// The Orderer group's Admins policy is what gates this update, so the orderer signs it.
	network.UpdateOrdererConfig(h.Network, h.Orderer, h.Channel, config, updated, h.Peer, h.Orderer)
}

// ConfigSequenceOn returns the sequence number of the channel configuration the
// named FSC node currently holds, as the node itself reports it.
//
// A configuration update returns as soon as the *ordering service* has
// committed it, which is strictly earlier than any FSC node observing it over
// delivery. Callers must therefore poll this with gomega.Eventually rather than
// asserting on it once. g asserts through the Gomega passed to the
// gomega.Eventually poll function, so a transient view-call failure is a
// retryable failure of that poll rather than an abort of the whole spec.
func ConfigSequenceOn(g gomega.Gomega, ii *integration.Infrastructure, node string) int {
	res, err := ii.Client(node).CallView("configseq", nil)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	return nwocommon.JSONUnmarshalInt(res)
}

// BatchTimeoutOf extracts the BatchTimeout from a channel configuration. It aborts the
// running spec through Gomega if the configuration does not carry one.
//
// The fabricx sibling suite shares it: the value is read the same way on both platforms.
func BatchTimeoutOf(config *common.Config) time.Duration {
	group, ok := config.ChannelGroup.Groups[OrdererGroupKey]
	gomega.Expect(ok).To(gomega.BeTrue(), "channel config has no [%s] group", OrdererGroupKey)

	value, ok := group.Values[BatchTimeoutKey]
	gomega.Expect(ok).To(gomega.BeTrue(), "[%s] group has no [%s] value", OrdererGroupKey, BatchTimeoutKey)

	batchTimeout := &protosorderer.BatchTimeout{}
	gomega.Expect(proto.Unmarshal(value.Value, batchTimeout)).NotTo(gomega.HaveOccurred())

	timeout, err := time.ParseDuration(batchTimeout.Timeout)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "cannot parse BatchTimeout [%s]", batchTimeout.Timeout)

	return timeout
}
