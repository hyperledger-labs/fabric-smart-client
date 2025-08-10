/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"testing"

	testing2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/testing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"

	"github.com/stretchr/testify/require"
)

func newBindingStoreForTests(tb testing.TB) *BindingStore {
	tb.Helper()

	pgConnStr := setupDB(tb)
	tb.Log("postgres ready")

	cp := NewConfigProvider(testing2.MockConfig(Config{
		DataSource: pgConnStr,
	}))
	db, err := NewPersistenceWithOpts(cp, NewDbProvider(), "", NewBindingStore)
	require.NoError(tb, err)
	return db
}

func TestPutBindingsMultipleEphemeralsPostgres(t *testing.T) {
	db := newBindingStoreForTests(t)
	common.TestPutBindingsMultipleEphemeralsCommon(t, db.BindingStore)
}

func TestManyManyPutBindingsPostgres(t *testing.T) {
	db := newBindingStoreForTests(t)
	common.TestManyManyPutBindingsCommon(t, db.BindingStore)
}
