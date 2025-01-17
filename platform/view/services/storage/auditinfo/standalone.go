/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package auditinfo

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
)

const (
	persistenceOptsConfigKey = "fsc.auditinfo.persistence.opts"
)

func NewWithConfig(dbDriver driver.Driver, namespace string, cp db.Config) (driver.AuditInfoPersistence, error) {
	return dbDriver.NewAuditInfo(namespace, db.NewPrefixConfig(cp, persistenceOptsConfigKey))
}
