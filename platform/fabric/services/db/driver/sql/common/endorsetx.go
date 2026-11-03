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

// NewEndorseTxStore returns an endorsement-transaction store over table, reading
// through readDB and writing through writeDB. Its queries use $N placeholders,
// which both SQLite and Postgres accept.
func NewEndorseTxStore(writeDB common.WriteDB, readDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper) *EndorseTxStore {
	return &EndorseTxStore{p: common.NewSimpleKeyDataStore(writeDB, readDB, table, errorWrapper)}
}

// EndorseTxStore is a write-once store of endorsed transactions by key.
type EndorseTxStore struct {
	p *common.SimpleKeyDataStore
}

func (db *EndorseTxStore) GetEndorseTx(ctx context.Context, key string) ([]byte, error) {
	return db.p.GetData(ctx, key)
}

func (db *EndorseTxStore) ExistsEndorseTx(ctx context.Context, key string) (bool, error) {
	return db.p.ExistData(ctx, key)
}

// PutEndorseTx stores data under key. Writing the value already stored under
// key returns nil; writing different bytes returns an error wrapping
// [driver.ErrConflict] and keeps the stored value.
func (db *EndorseTxStore) PutEndorseTx(ctx context.Context, key string, data []byte) error {
	return db.p.PutDataStrict(ctx, key, data)
}

func (db *EndorseTxStore) CreateSchema() error {
	return db.p.CreateSchema()
}

// StoreName returns the table name; it identifies the store in the versions table.
func (db *EndorseTxStore) StoreName() string { return db.p.StoreName() }

// Steps returns the store's schema steps. Step 1 creates the table if it does not exist.
func (db *EndorseTxStore) Steps() []migrate.Step { return db.p.Steps() }
