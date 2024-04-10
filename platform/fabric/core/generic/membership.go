/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	api2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/msp"
	"github.com/pkg/errors"
)

type ChannelMembershipService struct {
	// ResourcesApplyLock is used to serialize calls to CommitConfig and bundle update processing.
	ResourcesApplyLock sync.Mutex
	// ResourcesLock is used to serialize access to resources
	ResourcesLock sync.RWMutex
	// resources is used to acquire configuration bundle resources.
	ChannelResources channelconfig.Resources
}

func NewChannelMembershipService() *ChannelMembershipService {
	return &ChannelMembershipService{}
}

// Resources returns the active Channel configuration bundle.
func (c *ChannelMembershipService) Resources() channelconfig.Resources {
	c.ResourcesLock.RLock()
	res := c.ChannelResources
	c.ResourcesLock.RUnlock()
	return res
}

func (c *ChannelMembershipService) IsValid(identity view.Identity) error {
	id, err := c.Resources().MSPManager().DeserializeIdentity(identity)
	if err != nil {
		return errors.Wrapf(err, "failed deserializing identity [%s]", identity.String())
	}

	return id.Validate()
}

func (c *ChannelMembershipService) GetVerifier(identity view.Identity) (api2.Verifier, error) {
	id, err := c.Resources().MSPManager().DeserializeIdentity(identity)
	if err != nil {
		return nil, errors.Wrapf(err, "failed deserializing identity [%s]", identity.String())
	}
	return id, nil
}

// GetMSPIDs retrieves the MSP IDs of the organizations in the current Channel
// configuration.
func (c *ChannelMembershipService) GetMSPIDs() []string {
	ac, ok := c.Resources().ApplicationConfig()
	if !ok || ac.Organizations() == nil {
		return nil
	}

	var mspIDs []string
	for _, org := range ac.Organizations() {
		mspIDs = append(mspIDs, org.MSPID())
	}

	return mspIDs
}

// MSPManager returns the msp.MSPManager that reflects the current Channel
// configuration. Users should not memoize references to this object.
func (c *ChannelMembershipService) MSPManager() driver.MSPManager {
	return &mspManager{FabricMSPManager: c.Resources().MSPManager()}
}

type FabricMSPManager interface {
	DeserializeIdentity(serializedIdentity []byte) (msp.Identity, error)
}

type mspManager struct {
	FabricMSPManager
}

func (m *mspManager) DeserializeIdentity(serializedIdentity []byte) (driver.MSPIdentity, error) {
	return m.FabricMSPManager.DeserializeIdentity(serializedIdentity)
}
