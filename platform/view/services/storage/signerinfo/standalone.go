/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package signerinfo

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
)

const (
	persistenceOptsConfigKey = "fsc.signerinfo.persistence.opts"
)

func NewWithConfig(dbDriver driver.Driver, namespace string, cp db.Config) (driver.SignerInfoPersistence, error) {
	return dbDriver.NewSignerInfo(fmt.Sprintf("%s_signer", namespace), db.NewPrefixConfig(cp, persistenceOptsConfigKey))
}
