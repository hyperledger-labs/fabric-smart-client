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
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/unversioned"
	"github.com/pkg/errors"
)

type Opts struct {
	badger.Options
	Path string
}

func NewDriver() driver.NamedDriver {
	return driver.NamedDriver{Name: BadgerPersistence, Driver: &Driver{}}
}

func NewFileDriver() driver.NamedDriver {
	return driver.NamedDriver{Name: FilePersistence, Driver: &Driver{}}
}

type Driver struct{}

func (d *Driver) NewKVS(dataSourceName string, config driver.Config) (driver.TransactionalUnversionedPersistence, error) {
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
	backend, err := OpenDB(*opts, config)
	if err != nil {
		return nil, err
	}
	return &unversioned.Transactional{TransactionalVersioned: backend}, nil
}

func (d *Driver) NewBinding(string, driver.Config) (driver.BindingPersistence, error) {
	panic("not implemented")
}

func (d *Driver) NewSignerInfo(string, driver.Config) (driver.SignerInfoPersistence, error) {
	panic("not implemented")
}

func (d *Driver) NewAuditInfo(string, driver.Config) (driver.AuditInfoPersistence, error) {
	panic("not implemented")
}

func (d *Driver) NewEndorseTx(string, driver.Config) (driver.EndorseTxPersistence, error) {
	panic("not implemented")
}

func (d *Driver) NewMetadata(string, driver.Config) (driver.MetadataPersistence, error) {
	panic("not implemented")
}

func (d *Driver) NewEnvelope(string, driver.Config) (driver.EnvelopePersistence, error) {
	panic("not implemented")
}

func (d *Driver) NewVault(string, driver.Config) (driver.VaultPersistence, error) {
	panic("not implemented")
}
