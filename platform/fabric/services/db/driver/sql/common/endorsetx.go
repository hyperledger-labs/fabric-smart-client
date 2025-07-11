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

func NewEndorseTxStore(writeDB common3.WriteDB, readDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper, ci common.CondInterpreter) *EndorseTxStore {
	return &EndorseTxStore{p: common3.NewSimpleKeyDataStore(writeDB, readDB, table, errorWrapper, ci)}
}

type EndorseTxStore struct {
	p *common3.SimpleKeyDataStore
}

func (db *EndorseTxStore) GetEndorseTx(ctx context.Context, key string) ([]byte, error) {
	return db.p.GetData(ctx, key)
}

func (db *EndorseTxStore) ExistsEndorseTx(ctx context.Context, key string) (bool, error) {
	return db.p.ExistData(ctx, key)
}

func (db *EndorseTxStore) PutEndorseTx(ctx context.Context, key string, data []byte) error {
	return db.p.PutData(ctx, key, data)
}

func (db *EndorseTxStore) CreateSchema() error {
	return db.p.CreateSchema()
}
