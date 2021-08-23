/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package interop

type (
	Member struct {
		Type  string   `json:"type,omitempty"`
		Value string   `json:"value,omitempty"`
		Chain []string `json:"chain,omitempty"`
	}

	Membership struct {
		SecurityDomain string             `json:"securityDomain,omitempty"`
		Members        map[string]*Member `json:"members,omitempty"`
	}
)
