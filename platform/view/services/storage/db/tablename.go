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
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/lazy"
)

var (
	validName = regexp.MustCompile(`^[a-zA-Z_]+$`) // Thread safe
	replacers = []*replacer{
		newReplacer("_", "__"),
		newReplacer("-", "_d"),
		newReplacer("\\.", "_f"),
	}
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

func (c *TableNameCreator) CreateTableName(tablePrefix, name string, params ...string) (string, error) {
	nc, err := c.formatterProvider.Get(tablePrefix)
	if err != nil {
		return "", err
	}

	return nc.Format(name, params...)
}

type replacer struct {
	regex *regexp.Regexp
	repl  string
}

type tableNameFormatter struct {
	prefix string
	r      *regexp.Regexp
}

// Format builds the table name for name, prefixed with the formatter's configured prefix and,
// when params are given, an identifier escaped from them so that different callers of the same
// name don't collide. It returns an error if params contain characters that can't be turned into
// a valid identifier, or if the resulting table name is invalid.
func (c *tableNameFormatter) Format(name string, params ...string) (string, error) {
	if len(params) > 0 {
		escaped, err := escapeForTableName(params...)
		if err != nil {
			return "", errors.Wrapf(err, "failed to build table name for [%s]", name)
		}
		name = fmt.Sprintf("%s_%s", escaped, name)
	}
	if !c.r.MatchString(name) {
		return "", errors.Errorf("invalid table name [%s]: only letters and underscores allowed", name)
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

// escapeForTableName joins params and replaces characters that are unsafe in a table name
// (".", "-", "_") with identifier-safe substitutes. It returns an error if the result still
// contains characters outside [a-zA-Z_].
func escapeForTableName(params ...string) (string, error) {
	name := strings.Join(params, "_")
	for _, r := range replacers {
		name = r.Escape(name)
	}
	if len(name) > 0 && !validName.MatchString(name) {
		return "", errors.Errorf("unsupported chars found: %s", name)
	}
	return name, nil
}
