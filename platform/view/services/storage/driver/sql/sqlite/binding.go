/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
)

type BindingStore struct {
	*common2.BindingStore
	table        string
	writeDB      common2.WriteDB
	errorWrapper driver.SQLErrorWrapper
}

func NewBindingStore(dbs *common3.RWDB, tables common2.TableNames) (*BindingStore, error) {
	return newBindingStore(dbs.ReadDB, NewRetryWriteDB(dbs.WriteDB), tables.Binding), nil
}

func newBindingStore(readDB *sql.DB, writeDB common2.WriteDB, table string) *BindingStore {
	errorWrapper := &ErrorMapper{}
	return &BindingStore{
		BindingStore: common2.NewBindingStore(readDB, writeDB, table, errorWrapper, NewConditionInterpreter()),
		table:        table,
		writeDB:      writeDB,
		errorWrapper: errorWrapper,
	}
}
