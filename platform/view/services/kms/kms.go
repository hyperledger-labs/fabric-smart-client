/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kms

import (
	"sort"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

var (
	driversMu sync.RWMutex
	drivers   = make(map[string]Driver)
)

// Driver models the key management interface
type Driver interface {
	// Load returns the signer, verifier and signing identity bound to the byte representation of passed pem encoded public key.
	Load(configProvider ConfigProvider, pemBytes []byte) (view.Identity, driver.Signer, driver.Verifier, error)
}

type ConfigProvider interface {
	GetPath(s string) string
	GetStringSlice(key string) []string
	TranslatePath(path string) string
}

// Register makes a kms driver available by the provided name.
// If Register is called twice with the same name or if driver is nil,
// it panics.
func Register(name string, driver Driver) {
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

// GetKMSDriver returns the registered drivers.
func GetKMSDriver(driverName string) (Driver, error) {
	dNames := Drivers()
	if len(dNames) == 0 {
		return nil, errors.New("no kms driver found")
	}
	driver, ok := drivers[driverName]
	if !ok {
		return nil, errors.New("no kms driver found with name" + driverName)

	}
	return driver, nil

}
