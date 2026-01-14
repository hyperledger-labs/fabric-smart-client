/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
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

type LocalMembership interface {
	DefaultIdentity() view.Identity
	AnonymousIdentity() (view.Identity, error)
	IsMe(ctx context.Context, id view.Identity) bool
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
	GetVerifier(identity view.Identity) (Verifier, error)
}

type MembershipService interface {
	ChannelMembership
	Update(env *common.Envelope) error
	//DryUpdate(env *common.Envelope) error
	OrdererConfig(cs ConfigService) (string, []*grpc.ConnectionConfig, error)
}
