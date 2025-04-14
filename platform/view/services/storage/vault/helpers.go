/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"fmt"
	"path"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/postgres"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/sqlite"
)

func OpenMemoryVault() (driver.VaultPersistence, error) {
	return (&mem.Driver{}).NewVault("", nil)
}

func OpenSqliteVault(key, tempDir string) (driver.VaultPersistence, error) {
	o := opts{
		dataSource: fmt.Sprintf("%s.sqlite", path.Join(tempDir, key)),
	}
	return common.NewPersistenceWithOpts[sqlite.DbOpts]("test_table", o, sqlite.NewVaultPersistence)
}

func OpenPostgresVault(name string) (driver.VaultPersistence, func(), error) {
	postgresConfig := postgres.DefaultConfig(fmt.Sprintf("%s-db", name))
	terminate, err := postgres.StartPostgresWithFmt([]*postgres.ContainerConfig{postgresConfig})
	if err != nil {
		return nil, nil, err
	}

	o := opts{
		dataSource:   postgresConfig.DataSource(),
		maxOpenConns: 50,
	}
	persistence, err := common.NewPersistenceWithOpts[postgres.DbOpts]("test_table", o, postgres.NewVaultPersistence)
	if err != nil {
		return nil, nil, err
	}
	return persistence, terminate, nil
}

type opts struct {
	dataSource   string
	maxOpenConns int
}

func (o opts) DataSource() string         { return o.dataSource }
func (o opts) SkipPragmas() bool          { return false }
func (o opts) SkipCreateTable() bool      { return false }
func (o opts) MaxOpenConns() int          { return o.maxOpenConns }
func (o opts) MaxIdleConns() int          { return common.DefaultMaxIdleConns }
func (o opts) MaxIdleTime() time.Duration { return common.DefaultMaxIdleTime }
