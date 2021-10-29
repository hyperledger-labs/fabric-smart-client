/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type IdentityOptions struct {
	EIDExtension bool
	AuditInfo    []byte
}

type GetIdentityFunc func(opts *IdentityOptions) (view.Identity, []byte, error)

type IdentityInfo struct {
	ID           string
	EnrollmentID string
	GetIdentity  GetIdentityFunc
}

type SigningIdentity interface {
	Serialize() ([]byte, error)
	Sign(msg []byte) ([]byte, error)
}

type LocalMembership interface {
	DefaultIdentity() view.Identity
	AnonymousIdentity() view.Identity
	IsMe(id view.Identity) bool
	DefaultSigningIdentity() SigningIdentity
	RegisterX509MSP(id string, path string, mspID string) error
	RegisterIdemixMSP(id string, path string, mspID string) error
	GetIdentityByID(id string) (view.Identity, error)
	GetIdentityInfoByLabel(mspType string, label string) *IdentityInfo
	GetIdentityInfoByIdentity(mspType string, id view.Identity) *IdentityInfo
	Refresh() error
}

type MSPIdentity interface {
	GetMSPIdentifier() string
	Validate() error
	Verify(message, sigma []byte) error
}

type MSPManager interface {
	DeserializeIdentity(serializedIdentity []byte) (MSPIdentity, error)
}

type ChannelMembership interface {
	GetMSPIDs() []string
	MSPManager() MSPManager
	IsValid(identity view.Identity) error
	GetVerifier(identity view.Identity) (driver.Verifier, error)
}
