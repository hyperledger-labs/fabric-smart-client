/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	testing2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/testing"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"

	// postgres2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
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
	// if os.Getenv("TEST_POSTGRES") != "true" {
	// 	t.Skip("set environment variable TEST_POSTGRES to true to include postgres test")
	// }
	// if testing.Short() {
	// 	t.Skip("skipping postgres test in short mode")
	// }

	t.Log("starting postgres")
	// terminate, pgConnStr, err := postgres2.StartPostgres(t, false)
	terminate, pgConnStr, err := StartPostgres(t, false)
	require.NoError(t, err)
	defer terminate()
	t.Log("postgres ready")

	cp := NewConfigProvider(testing2.MockConfig(Config{
		DataSource: pgConnStr,
	}))
	db, err := NewPersistenceWithOpts(cp, NewDbProvider(), "", NewBindingStore)
	require.NoError(t, err)

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
