/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"context"
	"fmt"
	"path"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPutBindingsMultipleEphemerals(t *testing.T) {
	// Create mock DB and mock expectations
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	// Wrap sqlmock's db into RWDB
	rwdb := &common3.RWDB{
		WriteDB: db,
		ReadDB:  db,
	}

	// Prepare table names
	tables := common2.TableNames{
		Binding: "bindings",
	}

	// Create store using constructor
	store, err := NewBindingStore(rwdb, tables)
	require.NoError(t, err)

	common2.PutBindings(t, store, mock)
}

func TestPutBindingsMultipleEphemeralsFull(t *testing.T) {
	tempDir := t.TempDir()
	o := Opts{
		DataSource: fmt.Sprintf("file:%s.sqlite?_pragma=busy_timeout(1000)", path.Join(tempDir, "benchmark")),
	}
	dbs := utils.MustGet(open(o))
	tables := common2.GetTableNames(o.TablePrefix, o.TableNameParams...)
	db := newBindingStore(dbs.ReadDB, dbs.WriteDB, tables.Binding)
	assert.NoError(t, db.CreateSchema())
	ctx := context.Background()

	// Input identities
	longTerm := view.Identity("long")
	e1 := view.Identity("eph1")
	e2 := view.Identity("eph2")

	// Check that store does not have bindings for e1 and e2
	lt, err := db.GetLongTerm(ctx, e1)
	require.NoError(t, err)
	require.ElementsMatch(t, len(lt), 0)
	lt, err = db.GetLongTerm(ctx, e2)
	require.NoError(t, err)
	require.ElementsMatch(t, len(lt), 0)

	// Create new bindings
	err = db.PutBindings(ctx, longTerm, e1, e2)
	require.NoError(t, err)

	// Check that the bindings where correctly written
	lt, err = db.GetLongTerm(ctx, e1)
	require.NoError(t, err)
	require.ElementsMatch(t, lt, longTerm)

	lt, err = db.GetLongTerm(ctx, e2)
	require.NoError(t, err)
	require.ElementsMatch(t, lt, longTerm)
}
