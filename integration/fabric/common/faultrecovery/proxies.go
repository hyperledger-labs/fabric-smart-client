/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package faultrecovery

import (
	"os"
	"strings"

	"github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	nwonode "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
)

// RewriteCore replaces each old address with its new one in the core.yaml of node, then
// applies patch, if any, to the parsed configuration. Each old address must occur exactly
// once in the file, so the replacement rewrites only the link it names. For a node with
// replicas, it rewrites the core.yaml of the first replica; see RewriteReplicaCore.
func RewriteCore(ii *integration.Infrastructure, node string, patch func(core map[string]any), oldnew ...string) {
	rewriteCoreOf(ii, Replica(ii, node), patch, oldnew...)
}

// RewriteReplicaCore is RewriteCore for the core.yaml of replica r.
func RewriteReplicaCore(ii *integration.Infrastructure, r *nwonode.Replica, patch func(core map[string]any), oldnew ...string) {
	rewriteCoreOf(ii, r, patch, oldnew...)
}

func rewriteCoreOf(ii *integration.Infrastructure, r *nwonode.Replica, patch func(core map[string]any), oldnew ...string) {
	node := r.UniqueName
	path := ii.FscPlatform.NodeConfigPath(r)
	raw, err := os.ReadFile(path)
	gomega.ExpectWithOffset(2, err).NotTo(gomega.HaveOccurred())
	cfg := string(raw)
	for i := 0; i < len(oldnew); i += 2 {
		gomega.ExpectWithOffset(2, strings.Count(cfg, oldnew[i])).To(gomega.Equal(1), "[%s] in core.yaml of [%s]", oldnew[i], node)
	}
	cfg = strings.NewReplacer(oldnew...).Replace(cfg)
	if patch != nil {
		var core map[string]any
		gomega.ExpectWithOffset(2, yaml.Unmarshal([]byte(cfg), &core)).To(gomega.Succeed())
		patch(core)
		out, err := yaml.Marshal(core)
		gomega.ExpectWithOffset(2, err).NotTo(gomega.HaveOccurred())
		cfg = string(out)
	}
	gomega.ExpectWithOffset(2, os.WriteFile(path, []byte(cfg), 0o600)).To(gomega.Succeed())
}

// ReplicaProxies are proxies in front of one link of each replica of a node, keyed by the
// replica's unique name, such as bob.0. A replica may need more than one proxy for the
// link, one per backend it can connect to.
type ReplicaProxies map[string][]*common.Proxy

// Partition partitions every proxy of replica; see common.Proxy.Partition.
func (p ReplicaProxies) Partition(replica string) {
	gomega.ExpectWithOffset(1, p[replica]).NotTo(gomega.BeEmpty(), "no proxy for replica [%s]", replica)
	for _, proxy := range p[replica] {
		proxy.Partition()
	}
}

// Heal heals every proxy of replica; see common.Proxy.Heal.
func (p ReplicaProxies) Heal(replica string) {
	for _, proxy := range p[replica] {
		proxy.Heal()
	}
}

// Close closes every proxy.
func (p ReplicaProxies) Close() {
	for _, proxies := range p {
		for _, proxy := range proxies {
			_ = proxy.Close()
		}
	}
}

// StartProxy starts a common.Proxy on a free local port that forwards to backend. It fails
// the test if the proxy does not start.
func StartProxy(backend string) *common.Proxy {
	p, err := common.StartProxy("127.0.0.1:0", backend)
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred())
	return p
}
