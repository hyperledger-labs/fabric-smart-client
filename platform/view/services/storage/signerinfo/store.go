/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package signerinfo

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/multiplexed"
)

func NewDefaultStore(cp driver.Config, d multiplexed.Driver) (driver.SignerInfoPersistence, error) {
	return d.NewSignerInfo(common.GetPersistenceName(cp, "fsc.signerinfo.persistence"), "default")
}
