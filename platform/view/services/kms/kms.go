/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kms

import (
	"sort"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kms/driver"
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

func GetKeyManagementService(name string) (*driver.Driver, error) {
	// TODO: support multiple external committers
	dNames := Drivers()
	if len(dNames) == 0 {
		return nil, errors.New("no kms driver found" )
	}
	return drivers[dNames[0]], nil

}

// // Config models the DB configuration
// type Config interface {
// 	// UnmarshalKey takes a single key and unmarshals it into a Struct
// 	UnmarshalKey(key string, rawVal interface{}) error
// }

// // Open returns a new persistence handle. Similarly to database/sql:
// // driverName is a string that describes the driver
// // dataSourceName describes the data source in a driver-specific format.
// // The returned connection is only used by one goroutine at a time.
// func Open(sp view.ServiceProvider, driverName, dataSourceName string, config Config) (driver.Persistence, error) {
// 	driversMu.RLock()
// 	driver, ok := drivers[driverName]
// 	driversMu.RUnlock()
// 	if !ok {
// 		return nil, errors.Errorf("driver [%s] not found", driverName)
// 	}
// 	d, err := driver.New(sp, dataSourceName, config)
// 	if err != nil {
// 		return nil, errors.Wrapf(err, "failed opening datasource [%s][%s[", driverName, dataSourceName)
// 	}
// 	return d, nil
// }


