/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package migrate_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
)

// Schemas joins every registered list, sorted by store and dialect; registering a name again
// replaces its list, so an SDK built twice is reported once.
func TestSchemas(t *testing.T) { //nolint:paralleltest // registers process-wide schemas
	list := func(stores ...string) func() []migrate.Schema {
		return func() []migrate.Schema {
			var out []migrate.Schema
			for _, s := range stores {
				out = append(out, migrate.Schema{Store: s, Dialect: migrate.Postgres}, migrate.Schema{Store: s, Dialect: migrate.SQLite})
			}
			return out
		}
	}
	migrate.RegisterSchemas("b", list("z"))
	migrate.RegisterSchemas("a", list("y"))
	migrate.RegisterSchemas("a", list("x"))
	var got []string
	for _, s := range migrate.Schemas() {
		got = append(got, s.Store+"/"+string(s.Dialect))
	}
	require.Equal(t, []string{"x/postgres", "x/sqlite", "z/postgres", "z/sqlite"}, got)

	require.Panics(t, func() {
		migrate.NewSchema("s", migrate.SQLite, func() (migrate.Versioned, error) { return nil, migrate.ErrPending })
	})
}
