/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package faultrecovery

import (
	"fmt"
	"time"

	"github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	fabricnetwork "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/network"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	fxnetwork "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/network"
)

// KeepaliveTime and KeepaliveTimeout are the gRPC client keepalive SetFaultProxies and
// SetReplicaNotificationProxies give bob's notification service. KeepaliveTime is the 60 s minimum ping interval the
// committer's sidecar enforces; a node that pings more often is disconnected. A silent
// partition of the notification stream is therefore detected within their sum.
const (
	KeepaliveTime    = 60 * time.Second
	KeepaliveTimeout = 10 * time.Second
)

// FaultProxies are the proxies SetFaultProxies puts in bob's links to the committer.
// Partition and Heal on one of them cut and restore that link without either side seeing
// it close.
type FaultProxies struct {
	// Notification forwards bob's notification stream to the committer's sidecar.
	Notification *common.Proxy
}

// Close closes the proxies.
func (p *FaultProxies) Close() {
	_ = p.Notification.Close()
}

// SetFaultProxies starts a proxy in front of bob's committer's notification service, points
// bob's core.yaml at it, and enables the gRPC client keepalive of bob's notification
// service (see KeepaliveTime). Every other link, bob's query service and alice's links
// included, is direct.
// It assumes bob has one replica. Call it after the network is generated and before it
// starts; the proxies forward until Close.
func SetFaultProxies(ii *integration.Infrastructure) *FaultProxies {
	n := nwofabricx.FxPlatform(ii).Network
	notification := bobNotificationAddress(n)
	p := &FaultProxies{
		Notification: faultrecovery.StartProxy(notification),
	}
	faultrecovery.RewriteCore(ii, "bob", proxyNotification(n, notification, p.Notification.Addr()))
	return p
}

// SetReplicaNotificationProxies starts, for each replica of bob, a proxy in front of the
// committer's notification service, points the replica's core.yaml at its own proxy, and
// enables its gRPC client keepalive (see KeepaliveTime), so that partitioning a proxy cuts
// the notification stream of that replica alone and the replica detects it. The query
// service stays direct. It returns the proxies by replica unique name. Call it after the
// network is generated and before it starts; the proxies forward until Close.
func SetReplicaNotificationProxies(ii *integration.Infrastructure) faultrecovery.ReplicaProxies {
	n := nwofabricx.FxPlatform(ii).Network
	notification := bobNotificationAddress(n)
	proxies := faultrecovery.ReplicaProxies{}
	for _, r := range ii.FscPlatform.Peers {
		if r.Name != "bob" {
			continue
		}
		proxy := faultrecovery.StartProxy(notification)
		proxies[r.UniqueName] = []*common.Proxy{proxy}
		faultrecovery.RewriteReplicaCore(ii, r, proxyNotification(n, notification, proxy.Addr()))
	}
	return proxies
}

// bobNotificationAddress returns the address of the notification service of Org2's
// committer, which bob, in Org2, uses.
func bobNotificationAddress(n *fxnetwork.Network) string {
	committer := n.Peer("Org2", Org2Committer)
	gomega.ExpectWithOffset(1, committer).NotTo(gomega.BeNil(), "no committer peer for bob")
	return fmt.Sprintf("127.0.0.1:%d", n.PeerPort(committer, fabricnetwork.ListenPort))
}

// proxyNotification returns a core.yaml patch that points bob's notification service at
// proxy instead of notification and enables its gRPC client keepalive. The notification and
// query services may list the same address, so the notification service is rewritten
// through its own key rather than by address.
func proxyNotification(n *fxnetwork.Network, notification, proxy string) func(core map[string]any) {
	return func(core map[string]any) {
		fabric, ok := core["fabric"].(map[string]any)[n.Topology().Name()].(map[string]any)
		gomega.Expect(ok).To(gomega.BeTrue(), "core.yaml of bob has no fabric.%s", n.Topology().Name())
		setEndpoint(fabric, "notificationService", notification, proxy)
		fabric["notificationService"].(map[string]any)["keepalive"] = map[string]any{
			"time":    KeepaliveTime.String(),
			"timeout": KeepaliveTimeout.String(),
		}
	}
}

// setEndpoint replaces the address of the one endpoint of service, which must be old,
// with addr.
func setEndpoint(fabric map[string]any, service, old, addr string) {
	svc, ok := fabric[service].(map[string]any)
	gomega.ExpectWithOffset(1, ok).To(gomega.BeTrue(), "core.yaml of bob has no %s", service)
	endpoints, ok := svc["endpoints"].([]any)
	gomega.ExpectWithOffset(1, ok && len(endpoints) == 1).To(gomega.BeTrue(), "%s of bob has one endpoint", service)
	endpoint, ok := endpoints[0].(map[string]any)
	gomega.ExpectWithOffset(1, ok).To(gomega.BeTrue())
	gomega.ExpectWithOffset(1, endpoint["address"]).To(gomega.Equal(old), "%s address of bob", service)
	endpoint["address"] = addr
}
