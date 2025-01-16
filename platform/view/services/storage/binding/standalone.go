/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package binding

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
)

const (
	persistenceOptsConfigKey = "fsc.binding.persistence.opts"
)

func NewWithConfig(dbDriver driver.Driver, namespace string, cp db.Config) (driver.BindingPersistence, error) {
	return dbDriver.NewBinding(namespace, db.NewPrefixConfig(cp, persistenceOptsConfigKey))
}
