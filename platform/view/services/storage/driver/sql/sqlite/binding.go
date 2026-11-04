/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	drivercommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	sqlcommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
)

type BindingStore struct {
	*sqlcommon.BindingStore
	table        string
	writeDB      sqlcommon.WriteDB
	errorWrapper driver.SQLErrorWrapper
}

func NewBindingStore(dbs *drivercommon.RWDB, tables sqlcommon.TableNames) (*BindingStore, error) {
	return buildBindingStore(dbs.ReadDB, NewRetryWriteDB(dbs.WriteDB), tables.Binding, sqlcommon.QueryTimeout(dbs.QueryTimeout)), nil
}

func buildBindingStore(readDB *sql.DB, writeDB sqlcommon.WriteDB, table string, timeout sqlcommon.QueryTimeout) *BindingStore {
	errorWrapper := &ErrorMapper{}
	return &BindingStore{
		BindingStore: sqlcommon.NewBindingStore(readDB, writeDB, table, errorWrapper, timeout),
		table:        table,
		writeDB:      writeDB,
		errorWrapper: errorWrapper,
	}
}
