/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"context"
	"database/sql"
	"runtime/debug"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

type WriteDB interface {
	Begin() (*sql.Tx, error)
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
	Exec(query string, args ...any) (sql.Result, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	Close() error
}

func InitSchema(db WriteDB, schemas ...string) (err error) {
	logger.Debug("creating tables")
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil && tx != nil {
			if err := tx.Rollback(); err != nil {
				logger.Errorf("failed to rollback [%s][%s]", err, debug.Stack())
			}
		}
	}()
	for _, schema := range schemas {
		logger.Debug(schema)
		if _, err = tx.Exec(schema); err != nil {
			return errors.Wrapf(err, "error creating schema: %s", schema)
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return err
}

func QueryUniqueContext[T any](ctx context.Context, db *sql.DB, query string, args ...any) (T, error) {
	logger.Debug(query, args)
	row := db.QueryRowContext(ctx, query, args...)
	var result T
	var err error
	if err = row.Scan(&result); err != nil && errors.Is(err, sql.ErrNoRows) {
		return result, nil
	}
	return result, err
}

// NewIterator returns an iterator over rows that decodes each row with scan.
//
// Next returns (nil, nil) once the result set is exhausted, and (nil, err) when
// reading rows failed, for example because the connection broke mid-result, so
// a truncated result is never mistaken for a complete one. The error is the
// driver's own, not mapped by a driver.SQLErrorWrapper; use
// NewWrappedIterator when callers match the storage driver sentinels.
// Close releases the underlying rows and must be called.
func NewIterator[V any](rows *sql.Rows, scan func(*V) error) iterators.Iterator[*V] {
	return &rowIterator[V]{rows: rows, scan: scan}
}

// NewWrappedIterator is NewIterator with every error from reading or scanning a
// row passed through errorWrapper, so callers can match the backend-independent
// sentinels of the storage driver package, such as driver.ErrConnectionLost for
// a connection lost mid-result.
func NewWrappedIterator[V any](rows *sql.Rows, scan func(*V) error, errorWrapper driver.SQLErrorWrapper) iterators.Iterator[*V] {
	return &rowIterator[V]{rows: rows, scan: scan, errorWrapper: errorWrapper}
}

type rowIterator[V any] struct {
	rows         *sql.Rows
	scan         func(*V) error
	errorWrapper driver.SQLErrorWrapper
}

func (it *rowIterator[V]) Close() {
	_ = it.rows.Close()
}

func (it *rowIterator[V]) Next() (*V, error) {
	if !it.rows.Next() {
		return nil, it.wrap(it.rows.Err())
	}
	var r V
	err := it.scan(&r)
	return &r, it.wrap(err)
}

func (it *rowIterator[V]) wrap(err error) error {
	if err == nil || it.errorWrapper == nil {
		return err
	}
	return it.errorWrapper.WrapError(err)
}
