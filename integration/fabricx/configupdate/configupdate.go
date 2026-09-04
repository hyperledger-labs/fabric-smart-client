/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package configupdate drives Fabric-X channel configuration changes against a running
// network, so that a test can observe how FSC nodes react to a configuration that changes
// while they are up.
//
// The FSC-side path under test is platform/fabricx/core/channel/config: a
// ChannelConfigMonitor polls the committer's query service for the latest configuration
// transaction and, on seeing a new one, hands the envelope to membership.Service.Update
// and reconfigures the ordering service from the new Orderer group. Until this suite
// existed that loop only ever ran for a channel's genesis configuration.
package configupdate

import (
	"context"
	"time"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	protosorderer "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric-x-common/common/channelconfig"
	"github.com/hyperledger/fabric-x-common/protoutil"
	"github.com/onsi/gomega"
	"google.golang.org/protobuf/proto"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	fabconfigupdate "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/configupdate"
	nwocommon "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	fxnetwork "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/network"
)

// AllNodes names the FSC nodes in [Topology]. Each runs its own channel config monitor and
// applies a configuration independently, so specs assert on all of them.
var AllNodes = []string{"borrower", "lender", "approver1", "approver2"}

// NetworkOf returns the network of the fabricx platform in the running infrastructure.
// It aborts the running spec unless the infrastructure holds exactly one.
func NetworkOf(ii *integration.Infrastructure) *fxnetwork.Network {
	platforms := ii.NWOCtx.PlatformsByType(nwofabricx.TopologyName)
	gomega.Expect(platforms).To(gomega.HaveLen(1), "expected exactly one fabricx platform")

	p, ok := platforms[0].(*nwofabricx.Platform)
	gomega.Expect(ok).To(gomega.BeTrue(), "expected the fabricx platform to be a *fabricx.Platform")

	return p.Network
}

// configSeqViewTimeout bounds a single configseq call. gomega.Eventually runs its poll
// function synchronously, so an unanswered view would block the polling loop itself rather
// than expire with it; a bound well inside the callers' polling window instead fails the
// current poll and leaves the loop free to retry. A node that has just been restarted is
// the case that needs the room.
const configSeqViewTimeout = 15 * time.Second

// ConfigSequenceOn returns the channel configuration sequence the named FSC node currently
// holds, as the node itself reports it through the configseq view.
//
// A configuration reaches a node on its config monitor's next poll, strictly later than
// the ordering service accepting it, so callers must poll this with gomega.Eventually
// rather than asserting on it once. Pass that poll's Gomega as g: a transient view-call
// failure then fails only the current poll, leaving the spec to retry, instead of
// aborting it.
func ConfigSequenceOn(g gomega.Gomega, ii *integration.Infrastructure, node string) int {
	ctx, cancel := context.WithTimeout(context.Background(), configSeqViewTimeout)
	defer cancel()

	res, err := ii.Client(node).CallViewWithContext(ctx, "configseq", nil)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	return nwocommon.JSONUnmarshalInt(res)
}

// BatchTimeout returns the orderer BatchTimeout in the channel configuration the committer
// currently holds. Like [fxnetwork.GetConfig], it queries the committer on every call.
func BatchTimeout(n *fxnetwork.Network) time.Duration {
	return fabconfigupdate.BatchTimeoutOf(fxnetwork.GetConfig(n))
}

// SetBatchTimeout submits a channel configuration update setting the orderer's
// BatchTimeout to the given value, and returns once the committer serves the resulting
// configuration, so a caller may read it back immediately.
//
// Passing the value the channel already holds aborts the running spec: an update that
// changes nothing cannot be computed.
//
// BatchTimeout is the value these tests change because it sits in the Orderer group, which
// is what the channel config monitor re-reads before reconfiguring the ordering service,
// and because the mock orderer ignores the value itself -- so changing it exercises the
// configuration path without disturbing the running network.
func SetBatchTimeout(n *fxnetwork.Network, timeout time.Duration) {
	current := fxnetwork.GetConfig(n)
	gomega.Expect(fabconfigupdate.BatchTimeoutOf(current)).NotTo(gomega.Equal(timeout),
		"BatchTimeout is already [%s]; an update that changes nothing cannot be computed", timeout)

	updated := cloneConfig(current)
	updated.ChannelGroup.Groups[fabconfigupdate.OrdererGroupKey].Values[fabconfigupdate.BatchTimeoutKey] = &cb.ConfigValue{
		ModPolicy: "Admins",
		Value:     protoutil.MarshalOrPanic(&protosorderer.BatchTimeout{Timeout: timeout.String()}),
	}

	submit(n, current, updated)
}

// submit sends an updated configuration through the topology's first orderer, co-signed by
// the peer-org Admins named in orgs, and returns once the committer serves a configuration
// newer than current.
//
// orgs is passed straight through to [fxnetwork.UpdateConfig]: pass none for an
// Orderer-group change, which the orderer's own admin signature already authorizes, or
// enough peer organizations to satisfy an Application- or Channel-group value's Admins
// policy.
//
// [fxnetwork.UpdateConfig] returns as soon as the ordering service accepts the envelope,
// which is roughly 15ms ahead of the committer applying it. Waiting here rather than at
// each call site means no caller can read back the configuration it just replaced.
func submit(n *fxnetwork.Network, current, updated *cb.Config, orgs ...string) {
	gomega.Expect(n.Channels).NotTo(gomega.BeEmpty(), "the fabricx topology declares no channel")
	gomega.Expect(n.Orderers).NotTo(gomega.BeEmpty(), "the fabricx topology declares no orderer")

	fxnetwork.UpdateConfig(n, n.Orderers[0], n.Channels[0].Name, current, updated, orgs...)

	gomega.Eventually(func() uint64 {
		return fxnetwork.GetConfig(n).GetSequence()
	}, 60*time.Second, time.Second).Should(gomega.BeNumerically(">", current.GetSequence()),
		"the committer never served a configuration newer than sequence [%d]", current.GetSequence())
}

// unsupportedCapability is a capability name no fabric-x binary defines. Requiring it in
// the Application group is refused by membership.capabilitiesSupported and accepted by the
// committer, which never checks capability support -- the asymmetry TestRefused needs.
const unsupportedCapability = "V99_0"

// RequireUnsupportedCapability submits a channel configuration update that adds an
// Application capability no node can support, and returns once the committer serves the
// resulting configuration. It aborts the running spec through Gomega if the channel
// configuration is not shaped the way this update assumes, or if the committer never
// serves the update.
//
// The Application group's Admins policy is MAJORITY Admins over its three organizations
// (Org1, Org2, Org3), so the update is co-signed by Org1 and Org2's peer Admins -- two of
// three -- alongside the orderer admin [submit] always signs with.
func RequireUnsupportedCapability(n *fxnetwork.Network) {
	current := fxnetwork.GetConfig(n)
	updated := cloneConfig(current)

	group, ok := updated.ChannelGroup.Groups[channelconfig.ApplicationGroupKey]
	gomega.Expect(ok).To(gomega.BeTrue(), "channel config has no [%s] group", channelconfig.ApplicationGroupKey)

	value, ok := group.Values[channelconfig.CapabilitiesKey]
	gomega.Expect(ok).To(gomega.BeTrue(), "[%s] group has no [%s] value",
		channelconfig.ApplicationGroupKey, channelconfig.CapabilitiesKey)

	capabilities := &cb.Capabilities{}
	gomega.Expect(proto.Unmarshal(value.Value, capabilities)).NotTo(gomega.HaveOccurred())
	gomega.Expect(capabilities.Capabilities).NotTo(gomega.HaveKey(unsupportedCapability),
		"the channel configuration already requires [%s], so adding it would change nothing",
		unsupportedCapability)

	if capabilities.Capabilities == nil {
		capabilities.Capabilities = map[string]*cb.Capability{}
	}
	capabilities.Capabilities[unsupportedCapability] = &cb.Capability{}

	group.Values[channelconfig.CapabilitiesKey] = &cb.ConfigValue{
		ModPolicy: value.ModPolicy,
		Value:     protoutil.MarshalOrPanic(capabilities),
	}

	submit(n, current, updated, "Org1", "Org2")
}

// cloneConfig returns a deep copy of a channel configuration, so that the original stays
// available as the "current" side of the update being computed.
func cloneConfig(config *cb.Config) *cb.Config {
	clone, ok := proto.Clone(config).(*cb.Config)
	gomega.Expect(ok).To(gomega.BeTrue(), "expected the cloned config to be a *common.Config")

	return clone
}
