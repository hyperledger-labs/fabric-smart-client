/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package db

import (
	"regexp"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/unversioned"
	"github.com/pkg/errors"
)

// Config models the DB configuration
type Config interface {
	// IsSet checks to see if the key has been set in any of the data locations
	IsSet(key string) bool
	// UnmarshalKey takes a single key and unmarshals it into a Struct
	UnmarshalKey(key string, rawVal interface{}) error
}

// Open returns a new persistence handle. Similarly to database/sql:
// driverName is a string that describes the driver
// dataSourceName describes the data source in a driver-specific format.
// The returned connection is only used by one goroutine at a time.
func Open(driver driver.Driver, dataSourceName string, config Config) (*UnversionedPersistence, error) {
	d, err := driver.NewUnversioned(dataSourceName, config)
	if err != nil {
		return nil, errors.Wrapf(err, "failed opening datasource [%s]", dataSourceName)
	}
	return &UnversionedPersistence{UnversionedPersistence: d}, nil
}

func OpenTransactional(driver driver.Driver, dataSourceName string, config Config) (*TransactionalUnversionedPersistence, error) {
	d, err := driver.NewTransactionalUnversioned(dataSourceName, config)
	if err != nil {
		return nil, errors.Wrapf(err, "failed opening datasource [%s]", dataSourceName)
	}
	return &TransactionalUnversionedPersistence{TransactionalUnversionedPersistence: d}, nil
}

// OpenVersioned returns a new *versioned* persistence handle. Similarly to database/sql:
// driverName is a string that describes the driver
// dataSourceName describes the data source in a driver-specific format.
// The returned connection is only used by one goroutine at a time.
func OpenVersioned(driver driver.Driver, dataSourceName string, config Config) (*VersionedPersistence, error) {
	d, err := driver.NewVersioned(dataSourceName, config)
	if err != nil {
		return nil, errors.Wrapf(err, "failed opening datasource [%s]", dataSourceName)
	}
	return &VersionedPersistence{VersionedPersistence: d}, nil
}

// Unversioned returns the unversioned persistence from the supplied versioned one
func Unversioned(store driver.VersionedPersistence) driver.UnversionedPersistence {
	return &unversioned.Unversioned{Versioned: store}
}

type UnversionedPersistence struct {
	driver.UnversionedPersistence
}

type VersionedPersistence struct {
	driver.VersionedPersistence
}

type TransactionalVersionedPersistence struct {
	driver.TransactionalVersionedPersistence
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
