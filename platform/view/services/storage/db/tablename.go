/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package db

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/lazy"
	"github.com/pkg/errors"
)

type tableNameFormatterProvider struct {
	lazy.Provider[string, *tableNameFormatter]
}

func NewTableNameCreator() tableNameFormatterProvider {
	return newTableNameCreatorWithDefaultPrefix("fsc")
}

func newTableNameCreatorWithDefaultPrefix(defaultPrefix string) tableNameFormatterProvider {
	return tableNameFormatterProvider{
		Provider: lazy.NewProvider(func(prefix string) (*tableNameFormatter, error) {
			if len(prefix) == 0 {
				prefix = defaultPrefix
			}
			if len(prefix) > 100 {
				return nil, errors.New("table prefix must be shorter than 100 characters")
			}
			return newTableNameFormatter(prefix)
		}),
	}
}

func (c *tableNameFormatterProvider) GetFormatter(prefix string) (*tableNameFormatter, error) {
	return c.Get(prefix)
}

// Sanitizer

var defaultSanitizer = newSanitizer(map[string]string{
	"_":   "__",
	"-":   "_d",
	"\\.": "_f",
})

type replacement struct {
	regex *regexp.Regexp
	repl  string
}

type sanitizer struct {
	replacers []replacement
}

func newSanitizer(replacementMap map[string]string) *sanitizer {
	rs := make([]replacement, 0, len(replacementMap))
	for escaped, repl := range replacementMap {
		rs = append(rs, replacement{
			regex: regexp.MustCompile(escaped),
			repl:  repl,
		})
	}
	return &sanitizer{replacers: rs}
}

// Formatter

func (e *sanitizer) Encode(name string) string {
	for _, r := range e.replacers {
		name = r.Escape(name)
	}
	return name
}

func newTableNameFormatter(prefix string) (*tableNameFormatter, error) {
	validName := regexp.MustCompile(`^[a-zA-Z_]+$`) // Thread safe
	if len(prefix) > 0 && !validName.MatchString(prefix) {
		return nil, errors.New("illegal character in table prefix, only letters and underscores allowed")
	}
	if len(prefix) > 0 {
		prefix = strings.ToLower(prefix) + "_"
	}

	return &tableNameFormatter{
		prefix:    prefix,
		validName: validName,
		sanitizer: defaultSanitizer,
	}, nil
}

type tableNameFormatter struct {
	prefix    string
	validName *regexp.Regexp
	sanitizer *sanitizer
}

func (c *tableNameFormatter) Format(name string, params ...string) (string, error) {
	if len(params) > 0 {
		name = fmt.Sprintf("%s_%s", c.escapeForTableName(params...), name)
	}

	if !c.validName.MatchString(name) {
		return "", fmt.Errorf("invalid table name [%s]: only letters and underscores allowed", name)
	}
	return fmt.Sprintf("%s%s", c.prefix, name), nil
}

func (c *tableNameFormatter) MustFormat(name string, params ...string) string {
	return utils.MustGet(c.Format(name, params...))
}
func (r *replacement) Escape(s string) string {
	return r.regex.ReplaceAllString(s, r.repl)
}

func (c *tableNameFormatter) escapeForTableName(params ...string) string {
	name := c.sanitizer.Encode(strings.Join(params, "_"))

	if len(name) > 0 && !c.validName.MatchString(name) {
		panic("unsupported chars found: " + name)
	}
	return name
}
