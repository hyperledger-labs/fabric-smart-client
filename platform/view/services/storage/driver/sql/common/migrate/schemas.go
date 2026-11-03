/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package migrate

import (
	"cmp"
	"slices"
	"strings"
	"sync"
)

// Schema is the steps of one store type in one dialect, as `node version --json` reports them.
type Schema struct {
	// Store names the store type, such as "kvs" or "vault".
	Store   string
	Dialect Dialect
	Steps   []Step
}

// NewSchema returns the schema of the store that newStore builds without a database, as store
// type store in dialect d. It panics if newStore fails: store constructors fail only on a
// database.
func NewSchema(store string, d Dialect, newStore func() (Versioned, error)) Schema {
	v, err := newStore()
	if err != nil {
		panic(err)
	}
	return Schema{Store: store, Dialect: d, Steps: v.Steps()}
}

var (
	schemasMu sync.Mutex
	schemas   = map[string]func() []Schema{}
)

// RegisterSchemas makes the schemas that list returns part of Schemas. An SDK calls it when it is
// built, so that a binary reports the stores of the SDKs it links even before any SDK installs.
// Registering under a name again replaces the earlier list, so an SDK built twice registers once.
func RegisterSchemas(name string, list func() []Schema) {
	schemasMu.Lock()
	defer schemasMu.Unlock()
	schemas[name] = list
}

// Schemas returns every registered schema, ordered by store and dialect.
func Schemas() []Schema {
	schemasMu.Lock()
	lists := make([]func() []Schema, 0, len(schemas))
	for _, l := range schemas {
		lists = append(lists, l)
	}
	schemasMu.Unlock()
	var out []Schema
	for _, l := range lists {
		out = append(out, l()...)
	}
	slices.SortFunc(out, func(a, b Schema) int {
		return cmp.Or(strings.Compare(a.Store, b.Store), strings.Compare(string(a.Dialect), string(b.Dialect)))
	})
	return out
}
