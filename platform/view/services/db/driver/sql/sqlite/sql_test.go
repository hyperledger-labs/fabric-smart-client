/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"fmt"
	"path"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	_ "modernc.org/sqlite"
)

func TestSqlite(t *testing.T) {
	tempDir := t.TempDir()
	common2.TestCases(t, func(name string) (*common2.VersionedPersistence, error) {
		p, err := NewVersionedPersistence(versionedOpts(name, tempDir), "test")
		assert.NoError(t, err)
		assert.NoError(t, p.CreateSchema())
		return p, nil
	}, func(name string) (*common2.UnversionedPersistence, error) {
		p, err := NewUnversionedPersistence(unversionedOpts(name, tempDir), "test")
		assert.NoError(t, err)
		assert.NoError(t, p.CreateSchema())
		return p, nil
	}, func(name string) (driver.UnversionedNotifier, error) {
		p, err := NewUnversionedPersistenceNotifier(unversionedOpts(name, tempDir), "test")
		assert.NoError(t, err)
		assert.NoError(t, p.Persistence.CreateSchema())
		return p, nil
	}, func(name string) (driver.VersionedNotifier, error) {
		p, err := NewVersionedPersistenceNotifier(versionedOpts(name, tempDir), "test")
		assert.NoError(t, err)
		assert.NoError(t, p.Persistence.CreateSchema())
		return p, nil
	})
}

func unversionedOpts(name string, tempDir string) common2.Opts {
	return common2.Opts{DataSource: fmt.Sprintf("file:%s.sqlite?_pragma=busy_timeout(1000)", path.Join(tempDir, name))}
}

func versionedOpts(name string, tempDir string) common2.Opts {
	return common2.Opts{DataSource: fmt.Sprintf("%s.sqlite", path.Join(tempDir, name))}
}
