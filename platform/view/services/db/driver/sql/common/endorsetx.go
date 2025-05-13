/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/common"
)

func NewEndorseTxStore(writeDB WriteDB, readDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper, ci common.CondInterpreter) *EndorseTxStore {
	return &EndorseTxStore{p: newSimpleKeyDataStore(writeDB, readDB, table, errorWrapper, ci)}
}

type EndorseTxStore struct {
	p *simpleKeyDataStore
}

func (db *EndorseTxStore) GetEndorseTx(key string) ([]byte, error) {
	return db.p.GetData(key)
}

func (db *EndorseTxStore) ExistsEndorseTx(key string) (bool, error) {
	return db.p.ExistData(key)
}

func (db *EndorseTxStore) PutEndorseTx(key string, data []byte) error {
	return db.p.PutData(key, data)
}

func (db *EndorseTxStore) CreateSchema() error {
	return db.p.CreateSchema()
}
