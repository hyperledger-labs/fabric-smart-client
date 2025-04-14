/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
)

func NewWithConfig(dbDrivers driver.NamedDrivers, cp driver.ConfigProvider, params ...string) (driver2.VaultStore, error) {
	o, err := cp.GetConfig("vault.persistence", "txcode", params...)
	if err != nil {
		return nil, err
	}
	dbDriver, err := dbDrivers.GetDriver(o.DriverType())
	if err != nil {
		return nil, err
	}

	return dbDriver.NewVault(o.TableName(), o.DbOpts())
}
