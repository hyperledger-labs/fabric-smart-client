/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"fmt"
	"path"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
)

func newBindingStoreForTests(tb testing.TB) *BindingStore {
	tb.Helper()

	tempDir := tb.TempDir()
	o := Opts{
		DataSource: fmt.Sprintf("file:%s.sqlite?_pragma=busy_timeout(1000)", path.Join(tempDir, "benchmark")),
	}
	dbs, err := open(o)
	require.NoError(tb, err)
	tables, err := common.GetTableNames(o.TablePrefix, o.TableNameParams...)
	require.NoError(tb, err)
	db := buildBindingStore(dbs.ReadDB, dbs.WriteDB, tables.Binding, 0)
	require.NoError(tb, db.CreateSchema())
	return db
}

func TestPutBindingsMultipleEphemeralsSqlite(t *testing.T) {
	t.Parallel()
	db := newBindingStoreForTests(t)
	common.TestPutBindingsMultipleEphemeralsCommon(t, db.BindingStore)
}

func TestManyManyPutBindingsSqlite(t *testing.T) {
	t.Parallel()
	db := newBindingStoreForTests(t)
	common.TestManyManyPutBindingsCommon(t, db.BindingStore)
}

func TestPutBindingsRefusesKnownLongTermAsEphemeralSqlite(t *testing.T) {
	t.Parallel()
	db := newBindingStoreForTests(t)
	common.TestPutBindingsRefusesKnownLongTermAsEphemeralCommon(t, db.BindingStore)
}

func TestPutBindingsConflictKeepsRowAndReturnsNilSqlite(t *testing.T) {
	t.Parallel()
	db := newBindingStoreForTests(t)
	common.TestPutBindingsConflictKeepsRowAndReturnsNilCommon(t, db.BindingStore)
}

func TestPutBindingsDoesNotFollowRemoteRedirectSqlite(t *testing.T) {
	t.Parallel()
	db := newBindingStoreForTests(t)
	common.TestPutBindingsDoesNotFollowRemoteRedirectCommon(t, db.BindingStore)
}

func TestPutBindingsDoesNotFollowPreUpgradeRedirectSqlite(t *testing.T) {
	t.Parallel()
	db := newBindingStoreForTests(t)
	common.TestPutBindingsDoesNotFollowPreUpgradeRedirectCommon(t, db.BindingStore)
}

func TestReplaceBindingReplacesPlantedRowSqlite(t *testing.T) {
	t.Parallel()
	db := newBindingStoreForTests(t)
	common.TestReplaceBindingReplacesPlantedRowCommon(t, db.BindingStore)
}

func TestPutBindingsConcurrentSameEphemeralSqlite(t *testing.T) {
	t.Parallel()
	db := newBindingStoreForTests(t)
	common.TestPutBindingsConcurrentSameEphemeralCommon(t, db.BindingStore)
}

func TestPutBindingsUnknownLongTermIsFirstWriteWinsSqlite(t *testing.T) {
	t.Parallel()
	db := newBindingStoreForTests(t)
	common.TestPutBindingsUnknownLongTermIsFirstWriteWinsCommon(t, db.BindingStore)
}

func TestReplaceBindingDoesNotFollowPlantedRedirectSqlite(t *testing.T) {
	t.Parallel()
	db := newBindingStoreForTests(t)
	common.TestReplaceBindingDoesNotFollowPlantedRedirectCommon(t, db.BindingStore)
}

func TestReplaceBindingDoesNotRepairPlantedLongTermRowSqlite(t *testing.T) {
	t.Parallel()
	db := newBindingStoreForTests(t)
	common.TestReplaceBindingDoesNotRepairPlantedLongTermRowCommon(t, db.BindingStore)
}
