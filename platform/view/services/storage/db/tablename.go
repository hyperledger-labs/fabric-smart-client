/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package db

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/lazy"
	"github.com/pkg/errors"
)

type TableNameCreator struct {
	formatterProvider lazy.Provider[string, *tableNameFormatter]
}

func NewTableNameCreator() *TableNameCreator {
	return &TableNameCreator{formatterProvider: lazy.NewProvider(func(prefix string) (*tableNameFormatter, error) {
		if len(prefix) > 100 {
			return nil, errors.New("table prefix must be shorter than 100 characters")
		}
		r := regexp.MustCompile("^[a-zA-Z_]+$")
		if len(prefix) == 0 {
			return &tableNameFormatter{r: r}, nil
		}

		if !r.MatchString(prefix) {
			return nil, errors.New("illegal character in table prefix, only letters and underscores allowed")
		}
		return &tableNameFormatter{
			prefix: strings.ToLower(prefix) + "_",
			r:      r,
		}, nil
	})}
}

func (c *TableNameCreator) CreateTableName(escapedName, tablePrefix string) (string, error) {
	if tablePrefix == "" {
		tablePrefix = "fsc"
	}
	nc, err := c.formatterProvider.Get(tablePrefix)
	if err != nil {
		return "", err
	}

	tableName, valid := nc.Format(escapedName)
	if !valid {
		return "", fmt.Errorf("invalid table name [%s]: only letters and underscores allowed", escapedName)
	}
	return tableName, nil
}

func CreateTableName(name string, params ...string) string {
	return fmt.Sprintf("%s_%s", escapeForTableName(params...), name)
}

var validName = regexp.MustCompile(`^[a-zA-Z_]+$`) // Thread safe
var replacers = []*replacer{
	newReplacer("_", "__"),
	newReplacer("-", "_d"),
	newReplacer("\\.", "_f"),
}

type replacer struct {
	regex *regexp.Regexp
	repl  string
}

type tableNameFormatter struct {
	prefix string
	r      *regexp.Regexp
}

func (c *tableNameFormatter) Format(name string) (string, bool) {
	if !c.r.MatchString(name) {
		return "", false
	}
	return fmt.Sprintf("%s%s", c.prefix, name), true
}

func newReplacer(escaped, repl string) *replacer {
	return &replacer{
		regex: regexp.MustCompile(escaped),
		repl:  repl,
	}
}
func (r *replacer) Escape(s string) string {
	return r.regex.ReplaceAllString(s, r.repl)
}

func escapeForTableName(params ...string) string {
	name := strings.Join(params, "_")
	for _, r := range replacers {
		name = r.Escape(name)
	}
	if len(name) > 0 && !validName.MatchString(name) {
		panic("unsupported chars found: " + name)
	}
	return name
}
