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
)

// NewEnvelopeStore returns an envelope store over table, reading through readDB
// and writing through writeDB. Its queries use $N placeholders, which both
// SQLite and Postgres accept.
func NewEnvelopeStore(writeDB common3.WriteDB, readDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper) *EnvelopeStore {
	return &EnvelopeStore{p: common3.NewSimpleKeyDataStore(writeDB, readDB, table, errorWrapper)}
}

type EnvelopeStore struct {
	p *common3.SimpleKeyDataStore
}

func (db *EnvelopeStore) GetEnvelope(ctx context.Context, key string) ([]byte, error) {
	return db.p.GetData(ctx, key)
}

func (db *EnvelopeStore) ExistsEnvelope(ctx context.Context, key string) (bool, error) {
	return db.p.ExistData(ctx, key)
}

func (db *EnvelopeStore) PutEnvelope(ctx context.Context, key string, data []byte) error {
	return db.p.PutData(ctx, key, data)
}

func (db *EnvelopeStore) CreateSchema() error {
	return db.p.CreateSchema()
}
