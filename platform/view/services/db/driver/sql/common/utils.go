/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"context"
	"database/sql"
	"runtime/debug"

	errors2 "github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
	"github.com/pkg/errors"
)

type WriteDB interface {
	Begin() (*sql.Tx, error)
	Exec(query string, args ...any) (sql.Result, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	Close() error
}

type Sanitizer interface {
	Encode(string) (string, error)
	Decode(string) (string, error)
}

func newSanitizer(s Sanitizer) *sanitizer {
	return &sanitizer{Sanitizer: s}
}

type sanitizer struct {
	Sanitizer
}

func (s *sanitizer) EncodeAll(params []any) ([]any, error) {
	encoded := make([]any, len(params))
	for i, param := range params {
		encoded[i] = param
		if param == nil {
			continue
		}
		p, ok := param.(string)
		if !ok {
			continue
		}
		p, err := s.Encode(p)
		if err != nil {
			return nil, err
		}
		encoded[i] = p
	}
	return encoded, nil
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
			return errors2.Wrapf(err, "error creating schema: %s", schema)
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return
}

func QueryUnique[T any](db *sql.DB, query string, args ...any) (T, error) {
	return QueryUniqueContext[T](context.Background(), db, query, args...)
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

func NewIterator[V any](rows *sql.Rows, scan func(*V) error) iterators.Iterator[*V] {
	return &rowIterator[V]{rows: rows, scan: scan}
}

type rowIterator[V any] struct {
	rows *sql.Rows
	scan func(*V) error
}

func (it *rowIterator[V]) Close() {
	_ = it.rows.Close()
}

func (it *rowIterator[V]) Next() (*V, error) {
	if !it.rows.Next() {
		return nil, nil
	}
	var r V
	err := it.scan(&r)
	return &r, err
}
