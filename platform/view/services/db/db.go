/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package db

import (
	"sort"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/unversioned"
	"github.com/pkg/errors"
)

var (
	driversMu sync.RWMutex
	drivers   = make(map[string]driver.Driver)
)

// Register makes a db driver available by the provided name.
// If Register is called twice with the same name or if driver is nil,
// it panics.
func Register(name string, driver driver.Driver) {
	driversMu.Lock()
	defer driversMu.Unlock()
	if driver == nil {
		panic("Register driver is nil")
	}
	if _, dup := drivers[name]; dup {
		panic("Register called twice for driver " + name)
	}
	drivers[name] = driver
}

// Drivers returns a sorted list of the names of the registered drivers.
func Drivers() []string {
	driversMu.RLock()
	defer driversMu.RUnlock()
	list := make([]string, 0, len(drivers))
	for name := range drivers {
		list = append(list, name)
	}
	sort.Strings(list)
	return list
}

// Config models the DB configuration
type Config interface {
	// IsSet checks to see if the key has been set in any of the data locations
	IsSet(key string) bool
	// UnmarshalKey takes a single key and unmarshals it into a Struct
	UnmarshalKey(key string, rawVal interface{}) error
}

// Open returns a new persistence handle. Similarly to database/sql:
// driverName is a string that describes the driver
// dataSourceName describes the data source in a driver-specific format.
// The returned connection is only used by one goroutine at a time.
func Open(sp view.ServiceProvider, driverName, dataSourceName string, config Config) (driver.Persistence, error) {
	driversMu.RLock()
	driver, ok := drivers[driverName]
	driversMu.RUnlock()
	if !ok {
		return nil, errors.Errorf("driver [%s] not found", driverName)
	}
	d, err := driver.New(sp, dataSourceName, config)
	if err != nil {
		return nil, errors.Wrapf(err, "failed opening datasource [%s][%s[", driverName, dataSourceName)
	}
	return d, nil
}

// OpenVersioned returns a new *versioned* persistence handle. Similarly to database/sql:
// driverName is a string that describes the driver
// dataSourceName describes the data source in a driver-specific format.
// The returned connection is only used by one goroutine at a time.
func OpenVersioned(sp view.ServiceProvider, driverName, dataSourceName string, config Config) (driver.VersionedPersistence, error) {
	driversMu.RLock()
	driver, ok := drivers[driverName]
	driversMu.RUnlock()
	if !ok {
		return nil, errors.Errorf("driver [%s] not found", driverName)
	}
	d, err := driver.NewVersioned(sp, dataSourceName, config)
	if err != nil {
		return nil, errors.Wrapf(err, "failed opening datasource [%s][%s[", driverName, dataSourceName)
	}
	return d, nil
}

// Unversioned returns the unversioned persistence from the supplied versioned one
func Unversioned(store driver.VersionedPersistence) driver.Persistence {
	return &unversioned.Unversioned{Versioned: store}
}
