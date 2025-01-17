/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package signerinfo

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
)

const (
	persistenceOptsConfigKey = "fsc.signerinfo.persistence.opts"
)

func NewWithConfig(dbDriver driver.Driver, namespace string, cp db.Config) (driver.SignerInfoPersistence, error) {
	return dbDriver.NewSignerInfo(namespace, db.NewPrefixConfig(cp, persistenceOptsConfigKey))
}
