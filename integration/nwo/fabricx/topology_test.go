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

	t.Run("one committer by default", func(t *testing.T) {
		t.Parallel()
		topo := NewDefaultTopology()
		topo.AddOrganizationsByName("Org1", "Org2")
		require.Equal(t, []string{"Org1/SC"}, committerNames(topo.AllCommitters()))
	})

	t.Run("a committer per org", func(t *testing.T) {
		t.Parallel()
		topo := NewDefaultTopology().EnableCommitterPerOrg()
		topo.AddOrganizationsByName("Org1", "Org2", "Org3")
		topo.AddCommitter("Org2", "Org2_committer_1")
		require.Equal(t, []string{
			"Org1/Org1_committer_0", "Org2/Org2_committer_0", "Org3/Org3_committer_0", "Org2/Org2_committer_1",
		}, committerNames(topo.AllCommitters()))
	})

	t.Run("a named first committer keeps its name and org", func(t *testing.T) {
		t.Parallel()
		topo := NewDefaultTopology().EnableCommitterPerOrg().
			WithCommitterName("committer").WithCommitterOrg("Org2").
			WithCommitterEnv("SC_ORDERER_BLOCK_SIZE", "10")
		topo.AddOrganizationsByName("Org1", "Org2")
		all := topo.AllCommitters()
		require.Equal(t, []string{"Org2/committer", "Org1/Org1_committer_0"}, committerNames(all))
		require.Equal(t, "10", all[1].EnvVars["SC_ORDERER_BLOCK_SIZE"], "extra committers take the first one's env")
	})
}
