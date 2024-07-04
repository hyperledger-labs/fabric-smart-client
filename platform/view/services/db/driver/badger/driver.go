/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package badger

import (
	"os"
	"path/filepath"

	"github.com/dgraph-io/badger/v3"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/pkg/errors"
)

type Opts struct {
	badger.Options
	Path string
}

func NewDriver() driver.NamedDriver {
	return driver.NamedDriver{Name: "badger", Driver: &Driver{}}
}

func NewFileDriver() driver.NamedDriver {
	return driver.NamedDriver{Name: "file", Driver: &Driver{}}
}

type Driver struct{}

// NewTransactionalVersionedPersistence returns a new TransactionalVersionedPersistence for the passed data source and config
func (o *Driver) NewTransactionalVersioned(dataSourceName string, config driver.Config) (driver.TransactionalVersionedPersistence, error) {
	opts := &Opts{}
	if err := config.UnmarshalKey("", opts); err != nil {
		return nil, errors.Wrapf(err, "failed getting opts")
	}
	if err := config.UnmarshalKey("", &opts.Options); err != nil {
		return nil, errors.Wrapf(err, "failed getting opts")
	}
	path := filepath.Join(opts.Path, dataSourceName)
	opts.Path = path
	logger.Infof("opening badger at [%s], opts [%v]", path, opts)
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, errors.Wrapf(err, "failed creating directory [%s]", path)
	}
	return OpenDB(*opts, config)
}

func (v *Driver) NewVersioned(dataSourceName string, config driver.Config) (driver.VersionedPersistence, error) {
	return NewVersionedPersistence(dataSourceName, config)
}

func (v *Driver) NewUnversioned(dataSourceName string, config driver.Config) (driver.UnversionedPersistence, error) {
	return NewUnversionedPersistence(dataSourceName, config)
}
