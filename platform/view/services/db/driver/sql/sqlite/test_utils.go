/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"fmt"
	"path"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/unversioned"
)

type TestDriver struct {
	Name    string
	TempDir string
}

func (t *TestDriver) NewKVS(dataSourceName string, config driver.Config) (driver.TransactionalUnversionedPersistence, error) {
	p, err := NewVersioned(dbOpts(t.Name, t.TempDir), "test")
	if err != nil {
		return nil, err
	}
	if err := p.CreateSchema(); err != nil {
		return nil, err
	}
	return &unversioned.Transactional{TransactionalVersioned: p}, nil
}

func (t *TestDriver) NewBinding(dataSourceName string, config driver.Config) (driver.BindingPersistence, error) {
	return NewBindingPersistence(dbOpts(t.Name, t.TempDir), "test")
}

func (t *TestDriver) NewSignerInfo(string, driver.Config) (driver.SignerInfoPersistence, error) {
	return NewSignerInfoPersistence(dbOpts(t.Name, t.TempDir), "test")
}

func (t *TestDriver) NewAuditInfo(string, driver.Config) (driver.AuditInfoPersistence, error) {
	return NewAuditInfoPersistence(dbOpts(t.Name, t.TempDir), "test")
}

func (t *TestDriver) NewEndorseTx(string, driver.Config) (driver.EndorseTxPersistence, error) {
	return NewEndorseTxPersistence(dbOpts(t.Name, t.TempDir), "test")
}

func (t *TestDriver) NewMetadata(string, driver.Config) (driver.MetadataPersistence, error) {
	return NewMetadataPersistence(dbOpts(t.Name, t.TempDir), "test")
}

func (t *TestDriver) NewEnvelope(string, driver.Config) (driver.EnvelopePersistence, error) {
	return NewEnvelopePersistence(dbOpts(t.Name, t.TempDir), "test")
}

func (t *TestDriver) NewVault(string, driver.Config) (driver.VaultPersistence, error) {
	return NewVaultPersistence(dbOpts(t.Name, t.TempDir), "test")
}

func dbOpts(name string, tempDir string) common2.Opts {
	maxIdleConns, maxIdleTime := 2, 1*time.Minute
	return common2.Opts{DataSource: fmt.Sprintf("file:%s.sqlite?_pragma=busy_timeout(1000)", path.Join(tempDir, name)), MaxIdleConns: &maxIdleConns, MaxIdleTime: &maxIdleTime}
}
