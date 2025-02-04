/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"database/sql"
	"fmt"
	"regexp"
	"runtime/debug"
	"strings"
	"time"

	errors2 "github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
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

type TableNameCreator struct {
	prefix string
	r      *regexp.Regexp
}

func NewTableNameCreator(prefix string) (*TableNameCreator, error) {
	if len(prefix) > 100 {
		return nil, errors.New("table prefix must be shorter than 100 characters")
	}
	r := regexp.MustCompile("^[a-zA-Z_]+$")
	if len(prefix) == 0 {
		return &TableNameCreator{r: r}, nil
	}

	if !r.MatchString(prefix) {
		return nil, errors.New("illegal character in table prefix, only letters and underscores allowed")
	}
	return &TableNameCreator{
		prefix: strings.ToLower(prefix) + "_",
		r:      r,
	}, nil
}

func (c *TableNameCreator) GetTableName(name string) (string, bool) {
	if !c.r.MatchString(name) {
		return "", false
	}
	return fmt.Sprintf("%s%s", c.prefix, name), true
}

func (c *TableNameCreator) MustGetTableName(name string) string {
	if !c.r.MatchString(name) {
		panic("invalid name: " + name)
	}
	return fmt.Sprintf("%s%s", c.prefix, name)
}

func InitSchema(db WriteDB, schemas ...string) (err error) {
	logger.Info("creating tables")
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
		logger.Info(schema)
		if _, err = tx.Exec(schema); err != nil {
			return errors2.Wrapf(err, "error creating schema: %s", schema)
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return
}

func GetOpts(config driver.Config, optsKey string) (*Opts, error) {
	opts := Opts{}
	if err := config.UnmarshalKey(optsKey, &opts); err != nil {
		return nil, fmt.Errorf("failed getting opts: %w", err)
	}
	if opts.Driver == "" {
		return nil, notSetError(optsKey + ".driver")
	}
	if opts.DataSource == "" {
		return nil, notSetError(optsKey + ".dataSource")
	}
	if opts.MaxIdleConns == nil {
		opts.MaxIdleConns = CopyPtr(DefaultMaxIdleConns) // go default
	}
	if opts.MaxIdleTime == nil {
		opts.MaxIdleTime = CopyPtr(DefaultMaxIdleTime)
	}

	return &opts, nil
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

func notSetError(key string) error {
	return fmt.Errorf(
		"either %s in core.yaml or the %s environment variable must be specified", key,
		strings.ToUpper("CORE_"+strings.ReplaceAll(key, ".", "_")),
	)
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

type PersistenceConstructor[V dbObject] func(Opts, string) (V, error)

func NewPersistenceWithOpts[V dbObject](dataSourceName string, opts Opts, constructor PersistenceConstructor[V]) (V, error) {
	nc, err := NewTableNameCreator(opts.TablePrefix)
	if err != nil {
		return utils.Zero[V](), err
	}

	table, valid := nc.GetTableName(dataSourceName)
	if !valid {
		return utils.Zero[V](), fmt.Errorf("invalid table name [%s]: only letters and underscores allowed: %w", table, err)
	}
	p, err := constructor(opts, table)
	if err != nil {
		return utils.Zero[V](), err
	}
	if !opts.SkipCreateTable {
		if err := p.CreateSchema(); err != nil {
			return utils.Zero[V](), err
		}
	}
	return p, nil
}
