/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mem

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
)

func NewDriver() driver.NamedDriver {
	return driver.NamedDriver{
		Name:   "memory",
		Driver: &Driver{},
	}
}

type Driver struct{}

// NewTransactionalVersionedPersistence returns a new TransactionalVersionedPersistence for the passed data source and config
func (v *Driver) NewTransactionalVersioned(string, driver.Config) (driver.TransactionalVersionedPersistence, error) {
	panic("not supported")
}

func (v *Driver) NewVersioned(string, driver.Config) (driver.VersionedPersistence, error) {
	return NewVersionedPersistence(), nil
}

func (v *Driver) NewUnversioned(string, driver.Config) (driver.UnversionedPersistence, error) {
	return NewUnversionedPersistence(), nil
}
