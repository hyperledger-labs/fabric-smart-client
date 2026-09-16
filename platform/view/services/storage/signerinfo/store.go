/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package signerinfo

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/multiplexed"
)

// NewDefaultStore constructs a SignerInfoStore backed by the persistence configured under
// "fsc.signerinfo.persistence", resolved through d.
func NewDefaultStore(cp driver.Config, d multiplexed.Driver) (driver.SignerInfoStore, error) {
	name, err := common.GetPersistenceName(cp, "fsc.signerinfo.persistence")
	if err != nil {
		return nil, err
	}
	return d.NewSignerInfo(name, "default")
}
