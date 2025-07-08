/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"fmt"
	"path"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/stretchr/testify/assert"
)

func BenchmarkReadExistingSqlite(b *testing.B) {
	db, err := newTestKeyValueStore(b.TempDir())
	assert.NoError(b, err)
	defer utils.IgnoreErrorFunc(db.Close)

	common2.ReadExisting(b, db)
}

func BenchmarkReadNonExistingSqlite(b *testing.B) {
	db, err := newTestKeyValueStore(b.TempDir())
	assert.NoError(b, err)
	defer utils.IgnoreErrorFunc(db.Close)

	common2.ReadNonExisting(b, db)
}

func BenchmarkWriteOneSqlite(b *testing.B) {
	db, err := newTestKeyValueStore(b.TempDir())
	assert.NoError(b, err)
	defer utils.IgnoreErrorFunc(db.Close)

	common2.WriteOne(b, db)
}

func BenchmarkWriteManySqlite(b *testing.B) {
	db, err := newTestKeyValueStore(b.TempDir())
	assert.NoError(b, err)
	defer utils.IgnoreErrorFunc(db.Close)

	common2.WriteMany(b, db)
}

func newTestKeyValueStore(dir string) (driver.KeyValueStore, error) {
	o := Opts{
		DataSource:   fmt.Sprintf("file:%s.sqlite?_pragma=busy_timeout(1000)", path.Join(dir, "benchmark")),
		MaxIdleConns: 2,
		MaxIdleTime:  2 * time.Minute,
	}
	p, err := NewKeyValueStore(utils.MustGet(open(o)), common2.GetTableNames(o.TablePrefix, o.TableNameParams...))
	if err != nil {
		return nil, err
	}
	if err := p.CreateSchema(); err != nil {
		return nil, err
	}
	return p, nil
}
