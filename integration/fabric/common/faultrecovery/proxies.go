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
)

// RewriteCore replaces each old address with its new one in the core.yaml of node, then
// applies patch, if any, to the parsed configuration. Each old address must occur exactly
// once in the file, so the replacement rewrites only the link it names.
func RewriteCore(ii *integration.Infrastructure, node string, patch func(core map[string]any), oldnew ...string) {
	path := ii.FscPlatform.NodeConfigPath(Replica(ii, node))
	raw, err := os.ReadFile(path)
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred())
	cfg := string(raw)
	for i := 0; i < len(oldnew); i += 2 {
		gomega.ExpectWithOffset(1, strings.Count(cfg, oldnew[i])).To(gomega.Equal(1), "[%s] in core.yaml of [%s]", oldnew[i], node)
	}
	cfg = strings.NewReplacer(oldnew...).Replace(cfg)
	if patch != nil {
		var core map[string]any
		gomega.ExpectWithOffset(1, yaml.Unmarshal([]byte(cfg), &core)).To(gomega.Succeed())
		patch(core)
		out, err := yaml.Marshal(core)
		gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred())
		cfg = string(out)
	}
	gomega.ExpectWithOffset(1, os.WriteFile(path, []byte(cfg), 0o600)).To(gomega.Succeed())
}

// StartProxy starts a common.Proxy on a free local port that forwards to backend. It fails
// the test if the proxy does not start.
func StartProxy(backend string) *common.Proxy {
	p, err := common.StartProxy("127.0.0.1:0", backend)
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred())
	return p
}
