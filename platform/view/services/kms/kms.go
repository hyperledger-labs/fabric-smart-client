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

// Register makes a kms driver available by the provided name.
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

type KMS struct {
	driver.Driver
}

// Get returns the registered drivers.
func Get(driverName string) (*KMS, error) {
	dNames := Drivers()
	if len(dNames) == 0 {
		return nil, errors.New("no kms driver found")
	}
	driver, ok := drivers[driverName]
	if !ok {
		return nil, errors.New("no kms driver found with name" + driverName)

	}
	return &KMS{Driver: driver}, nil
}
