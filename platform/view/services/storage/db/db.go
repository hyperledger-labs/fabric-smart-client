/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package db

import (
	"regexp"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
)

// Config models the DB configuration
type Config interface {
	// IsSet checks to see if the key has been set in any of the data locations
	IsSet(key string) bool
	// UnmarshalKey takes a single key and unmarshals it into a Struct
	UnmarshalKey(key string, rawVal interface{}) error
}

type TransactionalUnversionedPersistence struct {
	driver.TransactionalUnversionedPersistence
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

func newReplacer(escaped, repl string) *replacer {
	return &replacer{
		regex: regexp.MustCompile(escaped),
		repl:  repl,
	}
}
func (r *replacer) Escape(s string) string {
	return r.regex.ReplaceAllString(s, r.repl)
}

func EscapeForTableName(params ...string) string {
	name := strings.Join(params, "_")
	for _, r := range replacers {
		name = r.Escape(name)
	}
	if len(name) > 0 && !validName.MatchString(name) {
		panic("unsupported chars found: " + name)
	}
	return name
}
