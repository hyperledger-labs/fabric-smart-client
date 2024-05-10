/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"fmt"
	"path"
	"testing"

	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

func TestSqlite(t *testing.T) {
	tempDir := t.TempDir()
	common2.TestCases(t, func(name string) (*common2.Persistence, error) {
		return initSqliteVersioned(tempDir, name)
	}, func(name string) (*common2.Unversioned, error) {
		return initSqliteUnversioned(tempDir, name)
	})
}
func initSqliteVersioned(tempDir, name string) (*common2.Persistence, error) {
	p, err := NewPersistence(common2.Opts{
		Driver:       "sqlite",
		DataSource:   fmt.Sprintf("%s.sqlite", path.Join(tempDir, name)),
		MaxOpenConns: 0,
		SkipPragmas:  false,
	}, "test")
	if err != nil {
		return nil, err
	}
	if err := p.CreateSchema(); err != nil {
		return nil, err
	}
	return p, nil
}
func initSqliteUnversioned(tempDir, name string) (*common2.Unversioned, error) {
	p, err := NewUnversioned(common2.Opts{
		Driver:       "sqlite",
		DataSource:   fmt.Sprintf("file:%s.sqlite?_pragma=busy_timeout(1000)", path.Join(tempDir, name)),
		MaxOpenConns: 0,
		SkipPragmas:  false,
	}, "test")
	if err != nil {
		return nil, err
	}
	if err := p.CreateSchema(); err != nil {
		return nil, fmt.Errorf("can't create schema: %w", err)
	}
	return p, nil
}
