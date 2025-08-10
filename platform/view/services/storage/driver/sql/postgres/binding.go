/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
)

type BindingStore struct {
	*common3.BindingStore
	table        string
	writeDB      *sql.DB
	errorWrapper driver.SQLErrorWrapper
}

func NewBindingStore(dbs *common2.RWDB, tables common3.TableNames) (*BindingStore, error) {
	return newBindingStore(dbs.ReadDB, dbs.WriteDB, tables.Binding), nil
}

func newBindingStore(readDB, writeDB *sql.DB, table string) *BindingStore {
	errorWrapper := &ErrorMapper{}
	return &BindingStore{
		BindingStore: common3.NewBindingStore(readDB, writeDB, table, errorWrapper, NewConditionInterpreter()),
		table:        table,
		writeDB:      writeDB,
		errorWrapper: errorWrapper,
	}
}
