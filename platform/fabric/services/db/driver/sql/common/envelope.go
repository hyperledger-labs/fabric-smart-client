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

// NewEnvelopeStore returns an envelope store over table, reading through readDB
// and writing through writeDB. Its queries use $N placeholders, which both
// SQLite and Postgres accept.
func NewEnvelopeStore(writeDB common.WriteDB, readDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper) *EnvelopeStore {
	return &EnvelopeStore{p: common.NewSimpleKeyDataStore(writeDB, readDB, table, errorWrapper)}
}

// EnvelopeStore is a write-once store of transaction envelopes by key.
type EnvelopeStore struct {
	p *common.SimpleKeyDataStore
}

func (db *EnvelopeStore) GetEnvelope(ctx context.Context, key string) ([]byte, error) {
	return db.p.GetData(ctx, key)
}

func (db *EnvelopeStore) ExistsEnvelope(ctx context.Context, key string) (bool, error) {
	return db.p.ExistData(ctx, key)
}

// PutEnvelope stores data under key. Writing the value already stored under
// key returns nil; writing a different one returns an error wrapping
// [driver.ErrConflict] and keeps the stored value.
func (db *EnvelopeStore) PutEnvelope(ctx context.Context, key string, data []byte) error {
	return db.p.PutDataStrict(ctx, key, data)
}

func (db *EnvelopeStore) CreateSchema() error {
	return db.p.CreateSchema()
}

// StoreName returns the table name; it identifies the store in the versions table.
func (db *EnvelopeStore) StoreName() string { return db.p.StoreName() }

// Steps returns the store's schema steps. Step 1 creates the table if it does not exist.
func (db *EnvelopeStore) Steps() []migrate.Step { return db.p.Steps() }
