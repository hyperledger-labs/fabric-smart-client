/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsc

import (
	"os"
	"testing"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	nwocontext "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/context"
)

// TestGenerateCoreConfig_LoadBalancer checks that alice resolves bob, whose replicas sit
// behind a load balancer, at the proxy's address and at no replica's own.
func TestGenerateCoreConfig_LoadBalancer(t *testing.T) { //nolint:paralleltest // the platform asserts through the global gomega
	gomega.RegisterTestingT(t)

	topology := NewTopology()
	topology.AddNodeByName("alice")
	topology.AddNodeByName("bob").AddOptions(WithReplicationFactor(2), WithLoadBalancer())
	p := NewPlatform(nwocontext.New(t.TempDir(), 20000, nil), topology, nil)
	lb := p.loadBalancers["bob"]
	require.NotEmpty(t, lb)

	p.GenerateResolverMap()
	p.GenerateCoreConfig(p.Peers[0])

	raw, err := os.ReadFile(p.NodeConfigPath(p.Peers[0]))
	require.NoError(t, err)
	var core struct {
		FSC struct {
			Endpoint struct {
				Resolvers []struct {
					Name      string            `yaml:"name"`
					Addresses map[string]string `yaml:"addresses"`
				} `yaml:"resolvers"`
			} `yaml:"endpoint"`
		} `yaml:"fsc"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &core))

	var addresses []string
	for _, r := range core.FSC.Endpoint.Resolvers {
		require.Equal(t, "bob", r.Name)
		addresses = append(addresses, r.Addresses[string(P2PPort)])
	}
	require.Equal(t, []string{lb, lb}, addresses)
}
