/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import "context"

// Resolver models a Fabric identity resolver
type Resolver struct {
	// context of the resolver
	Ctx context.Context
	// Name of the resolver
	Name string `yaml:"name,omitempty"`
	// Domain is option
	Domain string `yaml:"domain,omitempty"`
	// Identity specifies an MSP Identity
	Identity MSP `yaml:"identity,omitempty"`
	// Addresses where to reach this identity
	Addresses map[string]string `yaml:"addresses,omitempty"`
	// Aliases is a list of alias for this resolver
	Aliases []string `yaml:"aliases,omitempty"`
}

type Endpoint struct {
	Resolvers []Resolver `yaml:"resolvers,omitempty"`
}
