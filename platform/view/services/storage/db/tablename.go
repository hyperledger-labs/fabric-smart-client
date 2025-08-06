/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package db

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/lazy"
)

type TableNameCreator struct {
	formatterProvider lazy.Provider[string, *tableNameFormatter]
}

func NewTableNameCreator(defaultPrefix string) *TableNameCreator {
	return &TableNameCreator{formatterProvider: lazy.NewProvider(func(prefix string) (*tableNameFormatter, error) {
		if len(prefix) > 100 {
			return nil, errors.New("table prefix must be shorter than 100 characters")
		}
		r := regexp.MustCompile("^[a-zA-Z_]+$")
		if len(prefix) == 0 {
			prefix = defaultPrefix
		}
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

func (c *TableNameCreator) GetFormatter(prefix string) (*tableNameFormatter, error) {
	return c.formatterProvider.Get(prefix)
}

func (c *TableNameCreator) MustGetTableName(tablePrefix, name string, params ...string) string {
	return utils.MustGet(c.CreateTableName(tablePrefix, name, params...))
}

func (c *TableNameCreator) CreateTableName(tablePrefix, name string, params ...string) (string, error) {
	if tablePrefix == "" {
		tablePrefix = "fsc"
	}
	nc, err := c.formatterProvider.Get(tablePrefix)
	if err != nil {
		return "", err
	}

	return nc.Format(name, params...)
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

func (c *tableNameFormatter) MustFormat(name string, params ...string) string {
	return utils.MustGet(c.Format(name, params...))
}

func (c *tableNameFormatter) Format(name string, params ...string) (string, error) {
	if len(params) > 0 {
		name = fmt.Sprintf("%s_%s", escapeForTableName(params...), name)
	}
	if !c.r.MatchString(name) {
		return "", fmt.Errorf("invalid table name [%s]: only letters and underscores allowed", name)
	}
	return fmt.Sprintf("%s%s", c.prefix, name), nil
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
