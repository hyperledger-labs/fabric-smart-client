/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package interop

type (
	Policy struct {
		Type     string   `json:"type,omitempty"`
		Criteria []string `json:"criteria,omitempty"`
	}

	Identifier struct {
		Pattern string  `json:"pattern,omitempty"`
		Policy  *Policy `json:"policy,omitempty"`
	}

	VerificationPolicy struct {
		SecurityDomain string        `json:"securityDomain,omitempty"`
		Identifiers    []*Identifier `json:"identifiers,omitempty"`
	}
)
