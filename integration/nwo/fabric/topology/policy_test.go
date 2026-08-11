/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package topology

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The rules must be byte-identical to what AddNamespaceWithUnanimity and
// AddNamespaceWithOneOutOfN produced before they were retired, spaces included.
func TestPolicyRules(t *testing.T) { //nolint:paralleltest
	for _, tc := range []struct { //nolint:paralleltest
		name   string
		policy EndorsementPolicy
		want   string
		orgs   []string
	}{
		{
			name:   "unanimity over two orgs",
			policy: Unanimity("Org1", "Org2"),
			want:   "AND ('Org1MSP.member','Org2MSP.member')",
			orgs:   []string{"Org1", "Org2"},
		},
		{
			name:   "unanimity over one org",
			policy: Unanimity("Org1"),
			want:   "AND ('Org1MSP.member')",
			orgs:   []string{"Org1"},
		},
		{
			name:   "one out of n",
			policy: OneOutOfN("Org1", "Org2"),
			want:   "OutOf (1, 'Org1MSP.member','Org2MSP.member')",
			orgs:   []string{"Org1", "Org2"},
		},
		{
			name:   "verbatim signature rule",
			policy: Signature("OR ('Org1MSP.member','Org2MSP.member')"),
			want:   "OR ('Org1MSP.member','Org2MSP.member')",
			orgs:   nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.policy.Rule())
			require.Equal(t, tc.orgs, tc.policy.Orgs())
		})
	}
}
