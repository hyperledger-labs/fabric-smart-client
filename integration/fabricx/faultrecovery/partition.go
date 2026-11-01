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
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/network"
)

// KeepaliveTime and KeepaliveTimeout are the gRPC client keepalive SetFaultProxies gives
// bob's notification service. KeepaliveTime is the 60 s minimum ping interval the
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
	// Query forwards bob's query service calls to the committer.
	Query *common.Proxy
}

// Close closes the proxies.
func (p *FaultProxies) Close() {
	_ = p.Notification.Close()
	_ = p.Query.Close()
}

// SetFaultProxies starts a proxy in front of the committer's notification and query
// services, points bob's core.yaml at them, and enables the gRPC client keepalive of bob's
// notification service (see KeepaliveTime). Every other link, alice's included, is direct.
// It assumes bob has one replica. Call it after the network is generated and before it
// starts; the proxies forward until Close.
func SetFaultProxies(ii *integration.Infrastructure) *FaultProxies {
	n := nwofabricx.FxPlatform(ii).Network
	committer := n.Peer(n.CommitterOrg, n.CommitterName)
	gomega.Expect(committer).NotTo(gomega.BeNil(), "no committer peer")
	notification := fmt.Sprintf("127.0.0.1:%d", n.PeerPort(committer, fabricnetwork.ListenPort))
	query := fmt.Sprintf("127.0.0.1:%d", n.PeerPort(committer, network.QueryServicePortName))

	p := &FaultProxies{
		Notification: faultrecovery.StartProxy(notification),
		Query:        faultrecovery.StartProxy(query),
	}
	// Both services are listed under their own key, so each is rewritten through it rather
	// than by address.
	faultrecovery.RewriteCore(ii, "bob", func(core map[string]any) {
		fabric, ok := core["fabric"].(map[string]any)[n.Topology().Name()].(map[string]any)
		gomega.Expect(ok).To(gomega.BeTrue(), "core.yaml of bob has no fabric.%s", n.Topology().Name())
		setEndpoint(fabric, "notificationService", notification, p.Notification.Addr())
		setEndpoint(fabric, "queryService", query, p.Query.Addr())
		fabric["notificationService"].(map[string]any)["keepalive"] = map[string]any{
			"time":    KeepaliveTime.String(),
			"timeout": KeepaliveTimeout.String(),
		}
	})
	return p
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
