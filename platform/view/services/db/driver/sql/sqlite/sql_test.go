/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	"github.com/stretchr/testify/assert"
	_ "modernc.org/sqlite"
)

func TestSqlite(t *testing.T) {
	tempDir := t.TempDir()
	common2.TestCases(t, func(name string) (driver.UnversionedPersistence, error) {
		p, err := NewUnversionedPersistence(dbOpts(name, tempDir), "test")
		assert.NoError(t, err)
		assert.NoError(t, p.CreateSchema())
		return p, nil
	}, func(name string) (driver.UnversionedNotifier, error) {
		p, err := NewUnversionedNotifier(dbOpts(name, tempDir), "test")
		assert.NoError(t, err)
		assert.NoError(t, p.Persistence.(*UnversionedPersistence).CreateSchema())
		return p, nil
	}, func(p driver.UnversionedPersistence) *common2.UnversionedPersistence {
		return p.(*UnversionedPersistence).UnversionedPersistence
	})
}

func TestGetSqliteDir(t *testing.T) {
	assert.Equal(t, "/test/dir", getDir("file:/test/dir/db.sqlite"))
	assert.Equal(t, "/test/dir", getDir("file:/test/dir/db.sqlite?_txlock=immediate"))
	assert.Equal(t, "/test/dir", getDir("file:/test/dir/db.sqlite?_txlock=immediate&key=val"))
	assert.Equal(t, "/test", getDir("/test/db.sqlite"))
	assert.Equal(t, "relative/path", getDir("relative/path/filename.db"))
}

func TestFolderDoesNotExistError(t *testing.T) {
	_, err := NewUnversionedPersistence(dbOpts("folder-does-not-exist", "/this/folder/does/not/exist"), "test")
	assert.Error(t, err, "error opening db: can't open sqlite database, does the folder exist?")
}
