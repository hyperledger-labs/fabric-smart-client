/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"fmt"
	"path"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/stretchr/testify/assert"
)

func newBindingStoreForTests(t *testing.T) *BindingStore {
	tempDir := t.TempDir()
	o := Opts{
		DataSource: fmt.Sprintf("file:%s.sqlite?_pragma=busy_timeout(1000)", path.Join(tempDir, "benchmark")),
	}
	dbs := utils.MustGet(open(o))
	tables := common.GetTableNames(o.TablePrefix, o.TableNameParams...)
	db := newBindingStore(dbs.ReadDB, dbs.WriteDB, tables.Binding)
	assert.NoError(t, db.CreateSchema())
	return db
}

func TestPutBindingsMultipleEphemeralsSqlite(t *testing.T) {
	db := newBindingStoreForTests(t)
	common.TestPutBindingsMultipleEphemeralsCommon(t, db.BindingStore)
}

func TestManyManyPutBindingsSqlite(t *testing.T) {
	db := newBindingStoreForTests(t)
	common.TestManyManyPutBindingsCommon(t, db.BindingStore)
}
