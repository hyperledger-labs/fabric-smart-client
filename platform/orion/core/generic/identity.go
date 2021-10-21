/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
)

type IdentityManager struct {
	identities      []*driver.Identity
	defaultIdentity string
}

func (i *IdentityManager) Me() string {
	return i.defaultIdentity
}

func (i *IdentityManager) Identity(id string) *driver.Identity {
	for _, identity := range i.identities {
		if identity.Name == id {
			return identity
		}
	}
	return nil
}
