/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"database/sql"
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	errors2 "github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/pkg/errors"
)

const (
	DefaultMaxIdleConns = 2
	DefaultMaxIdleTime  = time.Minute
)

type WriteDB interface {
	Begin() (*sql.Tx, error)
	Exec(query string, args ...any) (sql.Result, error)
	Close() error
}

type Sanitizer interface {
	Encode(string) (string, error)
	Decode(string) (string, error)
}

type decoder interface {
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
	logger.Debug(query, args)
	row := db.QueryRow(query, args...)
	var result T
	var err error
	if err = row.Scan(&result); err != nil && errors.Is(err, sql.ErrNoRows) {
		return result, nil
	}
	return result, err
}

func CopyPtr[T any](t T) *T {
	v := t
	return &v
}

func GenerateParamSet(offset int, rows, cols int) string {
	sb := strings.Builder{}

	for i := 0; i < rows; i++ {
		if i > 0 {
			sb.WriteRune(',')
		}
		sb.WriteRune('(')
		for j := 0; j < cols; j++ {
			if j > 0 {
				sb.WriteRune(',')
			}
			sb.WriteString(fmt.Sprintf("$%d", offset))
			offset++
		}
		sb.WriteString(")")
	}

	return sb.String()
}

type dbObject interface {
	CreateSchema() error
}

type dbOpts interface {
	SkipCreateTable() bool
}

type PersistenceConstructor[O any, V dbObject] func(O, string) (V, error)

func NewPersistenceWithOpts[O dbOpts, V dbObject](table string, opts O, constructor PersistenceConstructor[O, V]) (V, error) {
	p, err := constructor(opts, table)
	if err != nil {
		return utils.Zero[V](), err
	}
	if !opts.SkipCreateTable() {
		if err := p.CreateSchema(); err != nil {
			return utils.Zero[V](), err
		}
	}
	return p, nil
}
