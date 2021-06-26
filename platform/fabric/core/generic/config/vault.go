/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

type VaultOpts struct {
	Path string `yaml:"path"`
}

type VaultPersistence struct {
	Type string    `yaml:"type"`
	Opts VaultOpts `yaml:"opts"`
}

type Vault struct {
	Persistence VaultPersistence `yaml:"persistence"`
}
