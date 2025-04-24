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
	"github.com/stretchr/testify/assert"
	_ "modernc.org/sqlite"
)

func TestSqlite(t *testing.T) {
	tempDir := t.TempDir()
	o := Opts{
		DataSource: fmt.Sprintf("file:%s.sqlite?_pragma=busy_timeout(1000)", path.Join(tempDir, "benchmark")),
	}
	common2.TestCases(t, func(name string) (driver.KeyValueStore, error) {
		p, err := NewKeyValueStore(o)
		assert.NoError(t, err)
		assert.NoError(t, p.CreateSchema())
		return p, nil
	}, func(name string) (driver.UnversionedNotifier, error) {
		p, err := NewKeyValueStoreNotifier(o, "test")
		assert.NoError(t, err)
		assert.NoError(t, p.Persistence.(*KeyValueStore).CreateSchema())
		return p, nil
	}, func(p driver.KeyValueStore) *common2.KeyValueStore {
		return p.(*KeyValueStore).KeyValueStore
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
	o := Opts{
		DataSource: fmt.Sprintf("file:%s.sqlite?_pragma=busy_timeout(1000)", path.Join("/this/folder/does/not/exist", "folder-does-not-exist")),
	}
	_, err := NewKeyValueStore(o)
	assert.Error(t, err, "error opening db: can't open sqlite database, does the folder exist?")
}
