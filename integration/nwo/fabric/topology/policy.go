/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package topology

import "strings"

// EndorsementPolicy is a namespace's chaincode endorsement policy. Construct it
// with Unanimity, OneOutOfN or Signature. Policies built from organization
// names also carry those names, which is how AddNamespace derives the peers
// that host the namespace.
type EndorsementPolicy struct {
	rule string
	orgs []string
}

// Rule returns the signature policy string handed to the lifecycle commands.
func (p EndorsementPolicy) Rule() string { return p.rule }

// Orgs returns the organizations the policy was built from, or nil for a
// verbatim Signature rule.
func (p EndorsementPolicy) Orgs() []string { return p.orgs }

// Unanimity requires an endorsement from every listed organization.
func Unanimity(orgs ...string) EndorsementPolicy {
	return EndorsementPolicy{rule: "AND (" + members(orgs) + ")", orgs: orgs}
}

// OneOutOfN requires an endorsement from any one of the listed organizations.
func OneOutOfN(orgs ...string) EndorsementPolicy {
	return EndorsementPolicy{rule: "OutOf (1, " + members(orgs) + ")", orgs: orgs}
}

// Signature takes a signature policy rule verbatim. Peers cannot be derived
// from it, so AddNamespace defaults them to every peer on the channel unless
// WithPeers says otherwise.
func Signature(rule string) EndorsementPolicy {
	return EndorsementPolicy{rule: rule}
}

func members(orgs []string) string {
	var b strings.Builder
	for i, org := range orgs {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("'" + org + "MSP.member'")
	}
	return b.String()
}
