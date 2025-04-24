/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
)

func NewEnvelopeStore(writeDB WriteDB, readDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper, ci Interpreter) *EnvelopeStore {
	return &EnvelopeStore{p: newSimpleKeyDataStore(writeDB, readDB, table, errorWrapper, ci)}
}

type EnvelopeStore struct {
	p *simpleKeyDataStore
}

func (db *EnvelopeStore) GetEnvelope(key string) ([]byte, error) {
	return db.p.GetData(key)
}

func (db *EnvelopeStore) ExistsEnvelope(key string) (bool, error) {
	return db.p.ExistData(key)
}

func (db *EnvelopeStore) PutEnvelope(key string, data []byte) error {
	return db.p.PutData(key, data)
}

func (db *EnvelopeStore) CreateSchema() error {
	return db.p.CreateSchema()
}
