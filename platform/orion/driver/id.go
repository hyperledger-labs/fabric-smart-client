/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

type Identity struct {
	Name string
	Cert string
	Key  string
}

type IdentityManager interface {
	Me() string
	Identity(id string) *Identity
}
