/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"context"
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
)

// NewMetadataStore returns a transaction-metadata store over table, reading
// through readDB and writing through writeDB. Its queries use $N placeholders,
// which both SQLite and Postgres accept.
func NewMetadataStore(writeDB common.WriteDB, readDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper, timeout common.QueryTimeout) *MetadataStore {
	return &MetadataStore{p: common.NewSimpleKeyDataStore(writeDB, readDB, table, errorWrapper, timeout)}
}

// MetadataStore stores transaction metadata by key. The first value written
// under a key wins.
type MetadataStore struct {
	p *common.SimpleKeyDataStore
}

func (db *MetadataStore) GetMetadata(ctx context.Context, key string) ([]byte, error) {
	return db.p.GetData(ctx, key)
}

func (db *MetadataStore) ExistMetadata(ctx context.Context, key string) (bool, error) {
	return db.p.ExistData(ctx, key)
}

// PutMetadata stores data under key unless key already has a value, in which
// case it keeps the stored value and returns nil. Transient data is stored
// again during multi-step endorsement, so a later write is not an error.
func (db *MetadataStore) PutMetadata(ctx context.Context, key string, data []byte) error {
	return db.p.PutData(ctx, key, data)
}

func (db *MetadataStore) CreateSchema() error {
	return db.p.CreateSchema()
}

// StoreName returns the table name; it identifies the store in the versions table.
func (db *MetadataStore) StoreName() string { return db.p.StoreName() }

// Steps returns the store's schema steps. Step 1 creates the table if it does not exist.
func (db *MetadataStore) Steps() []migrate.Step { return db.p.Steps() }
