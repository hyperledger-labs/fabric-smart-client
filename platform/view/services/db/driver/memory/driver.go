/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mem

import (
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/unversioned"
)

type Driver struct{}

func (v *Driver) NewVersioned(sp view2.ServiceProvider, dataSourceName string, config driver.Config) (driver.VersionedPersistence, error) {
	return New(), nil
}

func (v *Driver) New(sp view2.ServiceProvider, dataSourceName string, config driver.Config) (driver.Persistence, error) {
	return &unversioned.Unversioned{Versioned: New()}, nil
}

func init() {
	db.Register("memory", &Driver{})
}
