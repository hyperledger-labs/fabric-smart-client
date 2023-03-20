/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mem

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/unversioned"
)

type Driver struct{}

// NewTransactionalVersionedPersistence returns a new TransactionalVersionedPersistence for the passed data source and config
func (v *Driver) NewTransactionalVersionedPersistence(sp view.ServiceProvider, dataSourceName string, config driver.Config) (driver.TransactionalVersionedPersistence, error) {
	panic("not supported")
}

func (v *Driver) NewVersioned(sp view.ServiceProvider, dataSourceName string, config driver.Config) (driver.VersionedPersistence, error) {
	return New(), nil
}

func (v *Driver) New(sp view.ServiceProvider, dataSourceName string, config driver.Config) (driver.Persistence, error) {
	return &unversioned.Unversioned{Versioned: New()}, nil
}

func init() {
	db.Register("memory", &Driver{})
}
