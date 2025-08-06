/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type IdentityOptions struct {
	IdemixEIDExtension bool
	AuditInfo          []byte
}

func CompileIdentityOptions(opts ...IdentityOption) (*IdentityOptions, error) {
	txOptions := &IdentityOptions{}
	for _, opt := range opts {
		if err := opt(txOptions); err != nil {
			return nil, err
		}
	}
	return txOptions, nil
}

type IdentityOption func(*IdentityOptions) error

func WithIdemixEIDExtension() IdentityOption {
	return func(o *IdentityOptions) error {
		o.IdemixEIDExtension = true
		return nil
	}
}

func WithAuditInfo(ai []byte) IdentityOption {
	return func(o *IdentityOptions) error {
		o.AuditInfo = ai
		return nil
	}
}

type GetIdentityFunc func(opts ...IdentityOption) (view.Identity, []byte, error)

type IdentityInfo struct {
	ID           string
	EnrollmentID string
	GetIdentity  GetIdentityFunc
}

type SigningIdentity interface {
	Serialize() ([]byte, error)
	Sign(msg []byte) ([]byte, error)
}

type LocalMembership struct {
	network driver.FabricNetworkService
}

func (s *LocalMembership) RegisterIdemixMSP(id string, path string, mspID string) error {
	return s.network.LocalMembership().RegisterIdemixMSP(id, path, mspID)
}

func (s *LocalMembership) RegisterX509MSP(id string, path string, mspID string) error {
	return s.network.LocalMembership().RegisterX509MSP(id, path, mspID)
}

func (s *LocalMembership) DefaultSigningIdentity() SigningIdentity {
	return s.network.LocalMembership().DefaultSigningIdentity()
}

func (s *LocalMembership) DefaultIdentity() view.Identity {
	return s.network.LocalMembership().DefaultIdentity()
}

func (s *LocalMembership) IsMe(ctx context.Context, id view.Identity) bool {
	return s.network.LocalMembership().IsMe(ctx, id)
}

func (s *LocalMembership) AnonymousIdentity() (view.Identity, error) {
	return s.network.LocalMembership().AnonymousIdentity()
}

func (s *LocalMembership) GetIdentityByID(id string) (view.Identity, error) {
	return s.network.LocalMembership().GetIdentityByID(id)
}

func (s *LocalMembership) GetIdentityInfoByLabel(mspType string, label string) *IdentityInfo {
	iInfo := s.network.LocalMembership().GetIdentityInfoByLabel(mspType, label)
	if iInfo == nil {
		return nil
	}
	return &IdentityInfo{
		ID:           iInfo.ID,
		EnrollmentID: iInfo.EnrollmentID,
		GetIdentity: func(opts ...IdentityOption) (view.Identity, []byte, error) {
			idOpts, err := CompileIdentityOptions(opts...)
			if err != nil {
				return nil, nil, err
			}
			return iInfo.GetIdentity(&driver.IdentityOptions{
				EIDExtension: idOpts.IdemixEIDExtension,
				AuditInfo:    idOpts.AuditInfo,
			})
		},
	}
}

func (s *LocalMembership) GetIdentityInfoByIdentity(mspType string, id view.Identity) *IdentityInfo {
	iInfo := s.network.LocalMembership().GetIdentityInfoByIdentity(mspType, id)
	if iInfo == nil {
		return nil
	}
	return &IdentityInfo{
		ID:           iInfo.ID,
		EnrollmentID: iInfo.EnrollmentID,
		GetIdentity: func(opts ...IdentityOption) (view.Identity, []byte, error) {
			idOpts, err := CompileIdentityOptions(opts...)
			if err != nil {
				return nil, nil, err
			}
			return iInfo.GetIdentity(&driver.IdentityOptions{
				EIDExtension: idOpts.IdemixEIDExtension,
			})
		},
	}
}

func (s *LocalMembership) Refresh() error {
	return s.network.LocalMembership().Refresh()
}

// Verifier is an interface which wraps the Verify method.
type Verifier = driver.Verifier

type MSPManager struct {
	ch driver.ChannelMembership
}

func (c *MSPManager) GetMSPIDs() []string {
	return c.ch.GetMSPIDs()
}

func (c *MSPManager) IsValid(identity view.Identity) error {
	return c.ch.IsValid(identity)
}

func (c *MSPManager) GetMSPIdentifier(sid []byte) (string, error) {
	id, err := c.ch.MSPManager().DeserializeIdentity(sid)
	if err != nil {
		return "", errors.Wrapf(err, "failed deserializing identity [%s]", view.Identity(sid).UniqueID())
	}
	return id.GetMSPIdentifier(), nil
}

func (c *MSPManager) GetVerifier(identity view.Identity) (Verifier, error) {
	return c.ch.GetVerifier(identity)
}
