/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
)

func NewEnvelopePersistence(writeDB *sql.DB, readDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper, ci Interpreter) *EnvelopePersistence {
	return &EnvelopePersistence{p: newSimpleKeyDataPersistence(writeDB, readDB, table, errorWrapper, ci)}
}

type EnvelopePersistence struct {
	p *simpleKeyDataPersistence
}

func (db *EnvelopePersistence) GetEnvelope(key string) ([]byte, error) {
	return db.p.GetData(key)
}

func (db *EnvelopePersistence) ExistsEnvelope(key string) (bool, error) {
	return db.p.ExistData(key)
}

func (db *EnvelopePersistence) PutEnvelope(key string, data []byte) error {
	return db.p.PutData(key, data)
}

func (db *EnvelopePersistence) CreateSchema() error {
	return db.p.CreateSchema()
}
