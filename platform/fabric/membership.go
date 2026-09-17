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

type IdentityInfo struct {
	ID           string
	EnrollmentID string
	GetIdentity  driver.GetIdentityFunc
}

type SigningIdentity interface {
	Serialize() ([]byte, error)
	Sign(msg []byte) ([]byte, error)
}

type LocalMembership struct {
	network driver.FabricNetworkService
}

func (s *LocalMembership) RegisterIdemixMSP(id, path, mspID string) error {
	return s.network.LocalMembership().RegisterIdemixMSP(id, path, mspID)
}

func (s *LocalMembership) RegisterX509MSP(id, path, mspID string) error {
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

func (s *LocalMembership) GetIdentityInfoByLabel(mspType, label string) *IdentityInfo {
	iInfo := s.network.LocalMembership().GetIdentityInfoByLabel(mspType, label)
	if iInfo == nil {
		return nil
	}
	return &IdentityInfo{
		ID:           iInfo.ID,
		EnrollmentID: iInfo.EnrollmentID,
		GetIdentity:  iInfo.GetIdentity,
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
		GetIdentity:  iInfo.GetIdentity,
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

// GetMSPIDs returns the MSP IDs of the organizations in the channel's current
// configuration. It fails while the channel has no configuration in force:
// callers racing node startup can detect that with
// errors.Is(err, driver.ErrNotInitialized), and a configuration that arrived and
// was refused with errors.Is(err, driver.ErrConfigRejected).
func (c *MSPManager) GetMSPIDs() ([]string, error) {
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
