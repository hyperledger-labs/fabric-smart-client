/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package badger

import (
	"os"
	"path/filepath"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/notifier"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/unversioned"
	"github.com/pkg/errors"
)

func NewVersionedPersistence(dataSourceName string, config driver.Config) (driver.VersionedPersistence, error) {
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

func NewVersionedPersistenceNotifier(dataSourceName string, config driver.Config) (driver.VersionedNotifier, error) {
	persistence, err := NewVersionedPersistence(dataSourceName, config)
	if err != nil {
		return nil, err
	}
	return notifier.NewVersioned(persistence), nil
}

func NewUnversionedPersistence(dataSourceName string, config driver.Config) (driver.Persistence, error) {
	db, err := NewVersionedPersistence(dataSourceName, config)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to create badger driver for [%s]", dataSourceName)
	}
	return &unversioned.Unversioned{Versioned: db}, nil
}

func NewUnversionedPersistenceNotifier(dataSourceName string, config driver.Config) (driver.UnversionedNotifier, error) {
	persistence, err := NewUnversionedPersistence(dataSourceName, config)
	if err != nil {
		return nil, err
	}
	return notifier.NewUnversioned(persistence), nil
}
