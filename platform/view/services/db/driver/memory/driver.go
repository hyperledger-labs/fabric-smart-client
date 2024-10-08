/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mem

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/unversioned"
)

const (
	MemoryPersistence driver2.PersistenceType = "memory"
)

var (
	opts = common.Opts{
		Driver:          "sqlite",
		DataSource:      "file::memory:?cache=shared",
		TablePrefix:     "memory",
		SkipCreateTable: false,
		SkipPragmas:     false,
		MaxOpenConns:    10,
	}
)

type Driver struct{}

func NewDriver() driver.NamedDriver {
	return driver.NamedDriver{
		Name:   MemoryPersistence,
		Driver: &Driver{},
	}
}

func (d *Driver) NewVersioned(dataSourceName string, config driver.Config) (driver.VersionedPersistence, error) {
	return d.NewTransactionalVersioned(dataSourceName, config)
}

func (d *Driver) NewTransactionalUnversioned(dataSourceName string, config driver.Config) (driver.TransactionalUnversionedPersistence, error) {
	backend, err := d.NewTransactionalVersioned(dataSourceName, config)
	if err != nil {
		return nil, err
	}
	return &unversioned.Transactional{TransactionalVersioned: backend}, nil
}

func (d *Driver) NewTransactionalVersioned(string, driver.Config) (driver.TransactionalVersionedPersistence, error) {
	return sql.NewPersistenceWithOpts(utils.GenerateUUIDOnlyLetters(), opts, sql.VersionedConstructors)
}

func (d *Driver) NewUnversioned(string, driver.Config) (driver.UnversionedPersistence, error) {
	return sql.NewPersistenceWithOpts(utils.GenerateUUIDOnlyLetters(), opts, sql.UnversionedConstructors)
}
