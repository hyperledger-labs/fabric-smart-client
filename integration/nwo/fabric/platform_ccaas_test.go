/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
)

func TestCCaaSBuilderPath(t *testing.T) {
	base := t.TempDir()
	bin := filepath.Join(base, "bin")
	builder := filepath.Join(base, "builders", "ccaas")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(builder, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAB_BINS", bin)

	got, ok := ccaasBuilderPath()
	if !ok {
		t.Fatalf("expected builder found at %s", builder)
	}
	if got != builder {
		t.Fatalf("got %s, want %s", got, builder)
	}

	// A separate temp dir whose sibling "builders/ccaas" was never created.
	// Note: reusing a path under the same base (e.g. filepath.Join(base,
	// "nonexistent")) would NOT exercise the not-found case, since
	// filepath.Dir would still resolve to base, whose builders/ccaas
	// already exists from the setup above.
	other := t.TempDir()
	t.Setenv("FAB_BINS", filepath.Join(other, "bin"))
	if _, ok := ccaasBuilderPath(); ok {
		t.Fatalf("expected not found when builders/ccaas absent")
	}

	t.Setenv("FAB_BINS", "")
	if _, ok := ccaasBuilderPath(); ok {
		t.Fatalf("expected not found when FAB_BINS is empty")
	}
}

func TestTopologyPredicates(t *testing.T) { //nolint:paralleltest
	ccaasOnly := &topology.Topology{Chaincodes: []*topology.ChannelChaincode{
		{Chaincode: topology.Chaincode{Image: "fsc-cc/base:latest"}},
	}}
	require.True(t, topologyHasCCaaSChaincode(ccaasOnly))
	require.False(t, topologyHasLegacyChaincode(ccaasOnly))

	legacyOnly := &topology.Topology{Chaincodes: []*topology.ChannelChaincode{
		{Chaincode: topology.Chaincode{Path: "github.com/acme/cc"}},
	}}
	require.False(t, topologyHasCCaaSChaincode(legacyOnly))
	require.True(t, topologyHasLegacyChaincode(legacyOnly))

	mixed := &topology.Topology{Chaincodes: []*topology.ChannelChaincode{
		{Chaincode: topology.Chaincode{Image: "fsc-cc/base:latest"}},
		{Chaincode: topology.Chaincode{Path: "github.com/acme/cc"}},
	}}
	require.True(t, topologyHasCCaaSChaincode(mixed))
	require.True(t, topologyHasLegacyChaincode(mixed))

	empty := &topology.Topology{}
	require.False(t, topologyHasCCaaSChaincode(empty))
	require.False(t, topologyHasLegacyChaincode(empty))
}
