/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package auditinfo

import (
	"fmt"

	driver4 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/db"
	"github.com/pkg/errors"
)

const (
	persistenceOptsConfigKey = "fsc.auditinfo.persistence.opts"
	persistenceTypeConfigKey = "fsc.auditinfo.persistence.type"
)

func NewWithConfig(dbDrivers []driver.NamedDriver, cp db.Config, params ...string) (driver.AuditInfoPersistence, error) {
	d, err := getDriver(dbDrivers, cp)
	if err != nil {
		return nil, err
	}
	return d.NewAuditInfo(fmt.Sprintf("%s_auditinfo", db.EscapeForTableName(params...)), storage.NewPrefixConfig(cp, persistenceOptsConfigKey))
}

func getDriver(dbDrivers []driver.NamedDriver, cp db.Config) (driver.Driver, error) {
	var driverName driver4.PersistenceType
	if err := cp.UnmarshalKey(persistenceTypeConfigKey, &driverName); err != nil {
		return nil, err
	}
	for _, d := range dbDrivers {
		if d.Name == driverName {
			return d.Driver, nil
		}
	}
	return &mem.Driver{}, nil
}
