/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package signerinfo

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/multiplexed"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/db"
)

func NewStore(cp driver.Config, d multiplexed.Driver, params ...string) (driver.SignerInfoPersistence, error) {
	return d.NewSignerInfo(db.CreateTableName("sign", params...), db.NewPrefixConfig(cp, "fsc.signerinfo.persistence"))
}
