/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabricx

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
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

	t.Run("added committers are numbered on after the per-org ones", func(t *testing.T) {
		t.Parallel()
		topo := NewDefaultTopology().AddCommitter("Org1").AddCommitter("Org1").AddCommitter("Org2", WithName("Marcus"))
		topo.AddOrganizationsByName("Org1", "Org2")
		require.Equal(t, []string{
			"Org1/Org1_committer_0", "Org2/Org2_committer_0",
			"Org1/Org1_committer_1", "Org1/Org1_committer_2", "Org2/Marcus",
		}, committerNames(topo.AllCommitters()))
	})

	t.Run("topology settings apply to every committer, options override them", func(t *testing.T) {
		t.Parallel()
		ports := api.Ports{"Listen": 7101}
		topo := NewDefaultTopology().
			WithCommitterImage("img:topo").
			WithCommitterEnv("SC_VC_LOGGING_LOGSPEC", "debug").
			AddCommitter("Org1", WithImage("img:own"), WithEnv("SC_QUERY_LOGGING_LOGSPEC", "debug"), WithHost("h"), WithPorts(ports))
		topo.AddOrganizationsByName("Org1")
		all := topo.AllCommitters()
		require.Len(t, all, 2)

		require.Equal(t, "img:topo", all[0].Image)
		require.Equal(t, map[string]string{"SC_VC_LOGGING_LOGSPEC": "debug"}, all[0].EnvVars)

		require.Equal(t, "img:own", all[1].Image)
		require.Equal(t, map[string]string{"SC_VC_LOGGING_LOGSPEC": "debug", "SC_QUERY_LOGGING_LOGSPEC": "debug"}, all[1].EnvVars)
		require.Equal(t, "h", all[1].Host)
		require.Equal(t, ports, all[1].Ports)

		// Merging into one committer's env leaves the topology's untouched.
		require.Equal(t, map[string]string{"SC_VC_LOGGING_LOGSPEC": "debug"}, topo.CommitterEnvVars)
	})
}
