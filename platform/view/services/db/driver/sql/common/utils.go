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
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/pkg/errors"
)

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

func InitSchema(db *sql.DB, schemas ...string) (err error) {
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
	if !config.IsSet(optsKey + ".maxIdleConns") {
		opts.MaxIdleConns = 2 // go default
	}
	if !config.IsSet(optsKey + ".maxIdleTime") {
		opts.MaxIdleTime = time.Minute
	}

	return &opts, nil
}

func notSetError(key string) error {
	return fmt.Errorf(
		"either %s in core.yaml or the %s environment variable must be specified", key,
		strings.ToUpper("CORE_"+strings.ReplaceAll(key, ".", "_")),
	)
}
