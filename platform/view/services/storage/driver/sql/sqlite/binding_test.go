/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
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
