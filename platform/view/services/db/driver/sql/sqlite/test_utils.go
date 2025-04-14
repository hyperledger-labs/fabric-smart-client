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
)

const TestDriverName = "testsqlite"

type TestDriver struct {
	opts opts
}

func NewTestDriver(name, tempDir string) driver.NamedDriver {
	return driver.NamedDriver{
		Name:   TestDriverName,
		Driver: &TestDriver{opts: opts{name, tempDir}},
	}
}

func (t *TestDriver) NewKVS(string, driver.DbOpts) (driver.UnversionedPersistence, error) {
	return NewUnversionedPersistence(t.opts, "test")
}

func (t *TestDriver) NewBinding(string, driver.DbOpts) (driver.BindingPersistence, error) {
	return NewBindingPersistence(t.opts, "test")
}

func (t *TestDriver) NewSignerInfo(string, driver.DbOpts) (driver.SignerInfoPersistence, error) {
	return NewSignerInfoPersistence(t.opts, "test")
}

func (t *TestDriver) NewAuditInfo(string, driver.DbOpts) (driver.AuditInfoPersistence, error) {
	return NewAuditInfoPersistence(t.opts, "test")
}

func (t *TestDriver) NewEndorseTx(string, driver.DbOpts) (driver.EndorseTxPersistence, error) {
	return NewEndorseTxPersistence(t.opts, "test")
}

func (t *TestDriver) NewMetadata(string, driver.DbOpts) (driver.MetadataPersistence, error) {
	return NewMetadataPersistence(t.opts, "test")
}

func (t *TestDriver) NewEnvelope(string, driver.DbOpts) (driver.EnvelopePersistence, error) {
	return NewEnvelopePersistence(t.opts, "test")
}

func (t *TestDriver) NewVault(string, driver.DbOpts) (driver.VaultPersistence, error) {
	return NewVaultPersistence(t.opts, "test")
}

var maxIdleConns, maxIdleTime = 2, 1 * time.Minute

type opts struct {
	name    string
	tempDir string
}

func (o opts) DataSource() string {
	return fmt.Sprintf("file:%s.sqlite?_pragma=busy_timeout(1000)", path.Join(o.tempDir, o.name))
}
func (o opts) SkipPragmas() bool          { return false }
func (o opts) SkipCreateTable() bool      { return false }
func (o opts) MaxOpenConns() int          { return 0 }
func (o opts) MaxIdleConns() int          { return maxIdleConns }
func (o opts) MaxIdleTime() time.Duration { return maxIdleTime }
