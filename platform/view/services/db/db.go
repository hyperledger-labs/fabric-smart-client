/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package db

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/badger"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/unversioned"
)

// Open returns a new persistence handle. Similarly to database/sql:
// driverName is a string that describes the driver
// dataSourceName describes the data source in a driver-specific format.
// The returned connection is only used by one goroutine at a time.
func Open(driverName, dataSourceName string) (driver.Persistence, error) {
	switch driverName {
	case "memory":
		return &unversioned.Unversioned{Versioned: mem.New()}, nil
	case "badger":
		db, err := badger.OpenDB(dataSourceName)
		if err != nil {
			return nil, err
		}
		return &unversioned.Unversioned{Versioned: db}, nil
	default:
		return nil, errors.Errorf("invalid driver name %s", driverName)
	}
}

// OpenVersioned returns a new *versioned* persistence handle. Similarly to database/sql:
// driverName is a string that describes the driver
// dataSourceName describes the data source in a driver-specific format.
// The returned connection is only used by one goroutine at a time.
func OpenVersioned(driverName, dataSourceName string) (driver.VersionedPersistence, error) {
	switch driverName {
	case "memory":
		return mem.New(), nil
	case "badger":
		return badger.OpenDB(dataSourceName)
	default:
		return nil, fmt.Errorf("invalid driver name %s", driverName)
	}
}

// Unversioned returns the unversioned persistence from the supplied versioned one
func Unversioned(store driver.VersionedPersistence) driver.Persistence {
	return &unversioned.Unversioned{Versioned: store}
}
