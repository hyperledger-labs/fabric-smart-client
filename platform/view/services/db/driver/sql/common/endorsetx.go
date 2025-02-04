/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
)

func NewEndorseTxPersistence(writeDB WriteDB, readDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper, ci Interpreter) *EndorseTxPersistence {
	return &EndorseTxPersistence{p: newSimpleKeyDataPersistence(writeDB, readDB, table, errorWrapper, ci)}
}

type EndorseTxPersistence struct {
	p *simpleKeyDataPersistence
}

func (db *EndorseTxPersistence) GetEndorseTx(key string) ([]byte, error) {
	return db.p.GetData(key)
}

func (db *EndorseTxPersistence) ExistsEndorseTx(key string) (bool, error) {
	return db.p.ExistData(key)
}

func (db *EndorseTxPersistence) PutEndorseTx(key string, data []byte) error {
	return db.p.PutData(key, data)
}

func (db *EndorseTxPersistence) CreateSchema() error {
	return db.p.CreateSchema()
}
