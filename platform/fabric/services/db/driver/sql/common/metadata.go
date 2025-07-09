/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"context"
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/query/common"
)

func NewMetadataStore(writeDB common3.WriteDB, readDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper, ci common.CondInterpreter) *MetadataStore {
	return &MetadataStore{p: common3.NewSimpleKeyDataStore(writeDB, readDB, table, errorWrapper, ci)}
}

type MetadataStore struct {
	p *common3.SimpleKeyDataStore
}

func (db *MetadataStore) GetMetadata(ctx context.Context, key string) ([]byte, error) {
	return db.p.GetData(ctx, key)
}

func (db *MetadataStore) ExistMetadata(ctx context.Context, key string) (bool, error) {
	return db.p.ExistData(ctx, key)
}

func (db *MetadataStore) PutMetadata(ctx context.Context, key string, data []byte) error {
	return db.p.PutData(ctx, key, data)
}

func (db *MetadataStore) CreateSchema() error {
	return db.p.CreateSchema()
}
