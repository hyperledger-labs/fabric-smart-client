/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"fmt"
	"path"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/unversioned"
)

type TestDriver struct {
	Name    string
	TempDir string
}

func (t *TestDriver) NewTransactionalVersioned(dataSourceName string, config driver.Config) (driver.TransactionalVersionedPersistence, error) {
	p, err := NewVersioned(unversionedOpts(t.Name, t.TempDir), "test")
	if err != nil {
		return nil, err
	}
	if err := p.CreateSchema(); err != nil {
		return nil, err
	}
	return p, nil
}

func (t *TestDriver) NewVersioned(dataSourceName string, config driver.Config) (driver.VersionedPersistence, error) {
	p, err := NewVersioned(versionedOpts(t.Name, t.TempDir), "test")
	if err != nil {
		return nil, err
	}
	if err := p.CreateSchema(); err != nil {
		return nil, err
	}
	return p, nil
}

func (t *TestDriver) NewUnversioned(dataSourceName string, config driver.Config) (driver.UnversionedPersistence, error) {
	p, err := NewUnversioned(unversionedOpts(t.Name, t.TempDir), "test")
	if err != nil {
		return nil, err
	}
	if err := p.CreateSchema(); err != nil {
		return nil, err
	}
	return p, nil
}

func (t *TestDriver) NewTransactionalUnversioned(dataSourceName string, config driver.Config) (driver.TransactionalUnversionedPersistence, error) {
	p, err := NewVersioned(unversionedOpts(t.Name, t.TempDir), "test")
	if err != nil {
		return nil, err
	}
	if err := p.CreateSchema(); err != nil {
		return nil, err
	}
	return &unversioned.Transactional{TransactionalVersioned: p}, nil
}

func (t *TestDriver) NewBinding(dataSourceName string, config driver.Config) (driver.BindingPersistence, error) {
	return NewBindingPersistence(unversionedOpts(t.Name, t.TempDir), "test")
}

func unversionedOpts(name string, tempDir string) common2.Opts {
	return common2.Opts{DataSource: fmt.Sprintf("file:%s.sqlite?_pragma=busy_timeout(1000)", path.Join(tempDir, name))}
}

func versionedOpts(name string, tempDir string) common2.Opts {
	return common2.Opts{DataSource: fmt.Sprintf("%s.sqlite", path.Join(tempDir, name))}
}
