/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"context"
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

// maxBusyRetries bounds how often a write rejected with SQLITE_BUSY is retried.
// Each attempt already waits up to busy_timeout inside SQLite, so no backoff is
// added between attempts.
const maxBusyRetries = 3

// retryWriteDB retries Exec and ExecContext statements that fail with SQLITE_BUSY,
// up to maxBusyRetries times. Begin and BeginTx are not decorated: replaying a
// statement inside a transaction means replaying the transaction, which only
// the caller can do.
type retryWriteDB struct {
	*sql.DB

	errorWrapper driver.SQLErrorWrapper
}

// NewRetryWriteDB wraps the write pool db so busy Exec statements are retried.
func NewRetryWriteDB(db *sql.DB) *retryWriteDB {
	return &retryWriteDB{
		DB:           db,
		errorWrapper: &ErrorMapper{},
	}
}

func (db *retryWriteDB) Exec(query string, args ...any) (sql.Result, error) {
	return db.ExecContext(context.Background(), query, args...)
}

// ExecContext runs the statement and returns the result of the last attempt.
func (db *retryWriteDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	res, err := db.DB.ExecContext(ctx, query, args...)
	for range maxBusyRetries {
		if err == nil || !errors.Is(db.errorWrapper.WrapError(err), driver.SqlBusy) {
			break
		}
		logger.WarnfContext(ctx, "sql busy. Retrying query [%s]...", query)
		res, err = db.DB.ExecContext(ctx, query, args...)
	}
	return res, err
}
