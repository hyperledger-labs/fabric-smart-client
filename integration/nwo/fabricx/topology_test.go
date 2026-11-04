/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabricx

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func committerNames(cs []CommitterConfig) []string {
	names := make([]string, len(cs))
	for i, c := range cs {
		names[i] = c.Org + "/" + c.Name
	}
	return names
}

func TestAllCommitters(t *testing.T) {
	t.Parallel()

	t.Run("a committer per org", func(t *testing.T) {
		t.Parallel()
		topo := NewDefaultTopology()
		topo.AddOrganizationsByName("Org1", "Org2", "Org3")
		require.Equal(t, []string{
			"Org1/Org1_committer_0", "Org2/Org2_committer_0", "Org3/Org3_committer_0",
		}, committerNames(topo.AllCommitters()))
	})

	t.Run("the first org's committer is the first", func(t *testing.T) {
		t.Parallel()
		topo := NewDefaultTopology()
		topo.AddOrganizationsByName("Alpha", "Beta")
		require.Equal(t, []string{"Alpha/Alpha_committer_0", "Beta/Beta_committer_0"}, committerNames(topo.AllCommitters()))
	})

	t.Run("added committers follow the per-org ones", func(t *testing.T) {
		t.Parallel()
		topo := NewDefaultTopology().AddCommitter("Org2", "Org2_committer_1")
		topo.AddOrganizationsByName("Org1", "Org2")
		require.Equal(t, []string{
			"Org1/Org1_committer_0", "Org2/Org2_committer_0", "Org2/Org2_committer_1",
		}, committerNames(topo.AllCommitters()))
	})

	t.Run("the first committer's settings apply to the others", func(t *testing.T) {
		t.Parallel()
		topo := NewDefaultTopology().WithCommitterOrg("Org2").WithCommitterEnv("SC_SIDECAR_LOGGING_LEVEL", "debug")
		topo.AddOrganizationsByName("Org1", "Org2")
		all := topo.AllCommitters()
		require.Equal(t, []string{"Org2/Org2_committer_0", "Org1/Org1_committer_0"}, committerNames(all))
		require.Equal(t, "debug", all[1].EnvVars["SC_SIDECAR_LOGGING_LEVEL"])
	})
}
