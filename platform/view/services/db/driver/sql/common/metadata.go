/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
)

func NewMetadataPersistence(writeDB *sql.DB, readDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper, ci Interpreter) *MetadataPersistence {
	return &MetadataPersistence{p: newSimpleKeyDataPersistence(writeDB, readDB, table, errorWrapper, ci)}
}

type MetadataPersistence struct {
	p *simpleKeyDataPersistence
}

func (db *MetadataPersistence) GetMetadata(key string) ([]byte, error) {
	return db.p.GetData(key)
}

func (db *MetadataPersistence) ExistMetadata(key string) (bool, error) {
	return db.p.ExistData(key)
}

func (db *MetadataPersistence) PutMetadata(key string, data []byte) error {
	return db.p.PutData(key, data)
}

func (db *MetadataPersistence) CreateSchema() error {
	return db.p.CreateSchema()
}
