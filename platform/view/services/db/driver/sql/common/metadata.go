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

func NewMetadataStore(writeDB WriteDB, readDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper, ci common.CondInterpreter) *MetadataStore {
	return &MetadataStore{p: newSimpleKeyDataStore(writeDB, readDB, table, errorWrapper, ci)}
}

type MetadataStore struct {
	p *simpleKeyDataStore
}

func (db *MetadataStore) GetMetadata(key string) ([]byte, error) {
	return db.p.GetData(key)
}

func (db *MetadataStore) ExistMetadata(key string) (bool, error) {
	return db.p.ExistData(key)
}

func (db *MetadataStore) PutMetadata(key string, data []byte) error {
	return db.p.PutData(key, data)
}

func (db *MetadataStore) CreateSchema() error {
	return db.p.CreateSchema()
}
