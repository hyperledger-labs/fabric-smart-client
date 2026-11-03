/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sql_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate/migratetest"
)

// TestSteps checks every Fabric store's steps against testdata/steps/<dialect>-<store>.steps,
// so a released step cannot change unnoticed, and that every golden file has a store in
// Schemas, so `node version --json` cannot miss one.
func TestSteps(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob(filepath.Join("testdata", "steps", "*.steps"))
	require.NoError(t, err)
	var golden, listed []string
	for _, f := range files {
		golden = append(golden, strings.TrimSuffix(filepath.Base(f), ".steps"))
	}
	for _, s := range sql.Schemas() {
		name := string(s.Dialect) + "-" + s.Store
		listed = append(listed, name)
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			migratetest.CheckSteps(t, filepath.Join("testdata", "steps", name+".steps"), s.Steps)
		})
	}
	require.ElementsMatch(t, golden, listed)
}
