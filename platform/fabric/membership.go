/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package fabric

import (
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type GetIdentityFunc func() (view.Identity, []byte, error)

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

func (s *LocalMembership) IsMe(id view.Identity) bool {
	return s.network.LocalMembership().IsMe(id)
}

func (s *LocalMembership) AnonymousIdentity() view.Identity {
	return s.network.LocalMembership().AnonymousIdentity()
}

func (s *LocalMembership) GetIdentityByID(id string) (view.Identity, error) {
	return s.network.LocalMembership().GetIdentityByID(id)
}

func (s *LocalMembership) GetIdentityInfoByLabel(mspType string, label string) *IdentityInfo {
	ii := s.network.LocalMembership().GetIdentityInfoByLabel(mspType, label)
	if ii == nil {
		return nil
	}
	return &IdentityInfo{
		ID:           ii.ID,
		EnrollmentID: ii.EnrollmentID,
		GetIdentity: func() (view.Identity, []byte, error) {
			return ii.GetIdentity()
		},
	}
}

func (s *LocalMembership) GetIdentityInfoByIdentity(mspType string, id view.Identity) *IdentityInfo {
	ii := s.network.LocalMembership().GetIdentityInfoByIdentity(mspType, id)
	if ii == nil {
		return nil
	}
	return &IdentityInfo{
		ID:           ii.ID,
		EnrollmentID: ii.EnrollmentID,
		GetIdentity: func() (view.Identity, []byte, error) {
			return ii.GetIdentity()
		},
	}
}

func (s *LocalMembership) Refresh() error {
	return s.network.LocalMembership().Refresh()
}

// Verifier is an interface which wraps the Verify method.
type Verifier interface {
	// Verify verifies the signature over the passed message.
	Verify(message, sigma []byte) error
}

type MSPManager struct {
	ch driver.Channel
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
