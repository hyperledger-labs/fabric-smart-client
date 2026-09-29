/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package topology

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAppendOrganization(t *testing.T) { //nolint:paralleltest
	org := &Organization{Name: "Org1"}

	for _, tc := range []struct { //nolint:paralleltest
		name            string
		topology        *Topology
		wantConsortiums [][]string
		wantProfiles    [][]string
	}{
		{
			name:            "no consortiums or profiles",
			topology:        &Topology{},
			wantConsortiums: [][]string{},
			wantProfiles:    [][]string{},
		},
		{
			name: "single consortium and profile",
			topology: &Topology{
				Consortiums: []*Consortium{{Name: "SampleConsortium"}},
				Profiles:    []*Profile{{Name: "OrgsChannel"}},
			},
			wantConsortiums: [][]string{{"Org1"}},
			wantProfiles:    [][]string{{"Org1"}},
		},
		{
			name: "organization joins every consortium and profile",
			topology: &Topology{
				Consortiums: []*Consortium{{Name: "ConsortiumA"}, {Name: "ConsortiumB"}},
				Profiles:    []*Profile{{Name: "Channel1"}, {Name: "Channel2"}},
			},
			wantConsortiums: [][]string{{"Org1"}, {"Org1"}},
			wantProfiles:    [][]string{{"Org1"}, {"Org1"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NotPanics(t, func() { tc.topology.AppendOrganization(org) })
			require.Equal(t, []*Organization{org}, tc.topology.Organizations)

			gotConsortiums := make([][]string, len(tc.topology.Consortiums))
			for i, consortium := range tc.topology.Consortiums {
				gotConsortiums[i] = consortium.Organizations
			}
			require.Equal(t, tc.wantConsortiums, gotConsortiums)

			gotProfiles := make([][]string, len(tc.topology.Profiles))
			for i, profile := range tc.topology.Profiles {
				gotProfiles[i] = profile.Organizations
			}
			require.Equal(t, tc.wantProfiles, gotProfiles)
		})
	}
}
