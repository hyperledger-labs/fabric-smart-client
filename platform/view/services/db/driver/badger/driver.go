/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package badger

import (
	"os"
	"path/filepath"

	"github.com/dgraph-io/badger/v3"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/unversioned"
	"github.com/pkg/errors"
)

type Opts struct {
	badger.Options
	Path string
}

type Driver struct{}

func (v *Driver) NewVersioned(sp view.ServiceProvider, dataSourceName string, config driver.Config) (driver.VersionedPersistence, error) {
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

func (v *Driver) New(sp view.ServiceProvider, dataSourceName string, config driver.Config) (driver.Persistence, error) {
	db, err := v.NewVersioned(sp, dataSourceName, config)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to create badger driver for [%s]", dataSourceName)
	}
	return &unversioned.Unversioned{Versioned: db}, nil
}

func init() {
	db.Register("badger", &Driver{})
}
