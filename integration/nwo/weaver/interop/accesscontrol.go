/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package interop

type (
	Rule struct {
		Principal     string `json:"principal,omitempty"`
		PrincipalType string `json:"principalType,omitempty"`
		Resource      string `json:"resource,omitempty"`
		Read          bool   `json:"read,omitempty"`
	}

	AccessControl struct {
		SecurityDomain string  `json:"securityDomain,omitempty"`
		Rules          []*Rule `json:"rules,omitempty"`
	}
)
