/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

// Identity models an Orion identity
type Identity struct {
	// Name of the resolver
	Name string `yaml:"name,omitempty"`
	Cert string `yaml:"cert,omitempty"`
	Key  string `yaml:"key,omitempty"`
}

type Identities struct {
	Identities []Identity `yaml:"identities,omitempty"`
}
