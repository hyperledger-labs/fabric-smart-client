/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package signerinfo

import (
	"errors"
	"fmt"

	driver4 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/db"
)

const (
	persistenceOptsConfigKey = "fsc.signerinfo.persistence.opts"
	persistenceTypeConfigKey = "fsc.signerinfo.persistence.type"
)

func NewWithConfig(dbDrivers []driver.NamedDriver, cp db.Config, params ...string) (driver.SignerInfoPersistence, error) {
	d, err := getDriver(dbDrivers, cp)
	if err != nil {
		return nil, err
	}
	return d.NewSignerInfo(fmt.Sprintf("%s_signerinfo", db.EscapeForTableName(params...)), storage.NewPrefixConfig(cp, persistenceOptsConfigKey))
}

var supportedStores = collections.NewSet(mem.MemoryPersistence, sql.SQLPersistence)

func getDriver(dbDrivers []driver.NamedDriver, cp db.Config) (driver.Driver, error) {
	var driverName driver4.PersistenceType
	if err := cp.UnmarshalKey(persistenceTypeConfigKey, &driverName); err != nil {
		return nil, err
	}
	if !supportedStores.Contains(driverName) {
		return nil, errors.New("unsupported store")
	}
	for _, d := range dbDrivers {
		if d.Name == driverName {
			return d.Driver, nil
		}
	}
	return nil, errors.New("driver not found")
}
