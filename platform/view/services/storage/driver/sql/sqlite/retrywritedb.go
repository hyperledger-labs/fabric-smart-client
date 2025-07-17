/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"context"
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/pkg/errors"
)

type retryWriteDB struct {
	*sql.DB

	errorWrapper driver.SQLErrorWrapper
}

func NewRetryWriteDB(db *sql.DB) *retryWriteDB {
	return &retryWriteDB{
		DB:           db,
		errorWrapper: &ErrorMapper{},
	}
}

func (db *retryWriteDB) Exec(query string, args ...any) (sql.Result, error) {
	return db.ExecContext(context.Background(), query, args...)
}

func (db *retryWriteDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	res, err := db.DB.ExecContext(ctx, query, args...)
	if err != nil && errors.Is(db.errorWrapper.WrapError(err), driver.SqlBusy) {
		// TODO: AF Maybe limit the amount of retries
		logger.WarnfContext(ctx, "sql busy. Retrying query [%s]...", query)
		return db.ExecContext(ctx, query, args...)
	}
	return res, err
}
