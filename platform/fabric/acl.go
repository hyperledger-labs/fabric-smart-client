/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

type ACLProvider struct {
	ch driver.ChannelMembership
}

func NewACLProvider(ch driver.ChannelMembership) *ACLProvider {
	return &ACLProvider{ch: ch}
}

// CheckACL checks the ACL for the resource for the Channel using the
// SignedProposal from which an id can be extracted for testing against a policy
func (p *ACLProvider) CheckACL(signedProp *SignedProposal) error {
	return p.ch.CheckACL(signedProp.s)
}
