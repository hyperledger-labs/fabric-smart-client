/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"github.com/hyperledger/fabric/msp"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

// GetMSPIDs retrieves the MSP IDs of the organizations in the current Channel
// configuration.
func (c *Channel) GetMSPIDs() []string {
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
func (c *Channel) MSPManager() driver.MSPManager {
	return &mspManager{MSPManager: c.Resources().MSPManager()}
}

type MSPManager interface {
	DeserializeIdentity(serializedIdentity []byte) (msp.Identity, error)
}

type mspManager struct {
	MSPManager
}

func (m *mspManager) DeserializeIdentity(serializedIdentity []byte) (driver.MSPIdentity, error) {
	return m.MSPManager.DeserializeIdentity(serializedIdentity)
}
