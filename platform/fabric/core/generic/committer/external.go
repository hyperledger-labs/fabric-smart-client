/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"sort"
	"sync"

	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer/driver"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var (
	driversMu sync.RWMutex
	drivers   = make(map[string]driver.Driver)
)

// Register makes a kvs driver available by the provided name.
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

type ExternalCommitter struct {
	c driver.Committer
}

func (c *ExternalCommitter) Status(txid string) (fdriver.ValidationCode, []string, []view.Identity, error) {
	if c.c == nil {
		return fdriver.Unknown, nil, nil, nil
	}
	return c.c.Status(txid)
}

func (c *ExternalCommitter) Validate(txid string) (fdriver.ValidationCode, error) {
	if c.c == nil {
		panic("no external committer defined, programming error")
	}
	return c.c.Validate(txid)
}

func (c *ExternalCommitter) CommitTX(txid string, block uint64, indexInBloc int) error {
	if c.c == nil {
		panic("no external committer defined, programming error")
	}
	return c.c.CommitTX(txid, block, indexInBloc)
}

func (c *ExternalCommitter) DiscardTX(txid string) error {
	if c.c == nil {
		panic("no external committer defined, programming error")
	}
	return c.c.DiscardTX(txid)
}

func GetExternalCommitter(name string, sp view2.ServiceProvider, vault driver.Vault) (*ExternalCommitter, error) {
	// TODO: support multiple external committers
	dNames := Drivers()
	if len(dNames) == 0 {
		return &ExternalCommitter{c: nil}, nil
	}
	c, err := drivers[dNames[0]].Open(name, sp, vault)
	if err != nil {
		return nil, errors.Wrapf(err, "failed opening external committer [%s]", dNames[0])
	}
	return &ExternalCommitter{c: c}, nil
}
