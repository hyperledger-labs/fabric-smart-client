/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRetryWriteDBExec checks Exec runs a statement, and an error that is not SQLITE_BUSY
// is returned without a retry.
func TestRetryWriteDBExec(t *testing.T) {
	t.Parallel()

	db := NewRetryWriteDB(openTestDB(t))

	_, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY)")
	require.NoError(t, err)

	_, err = db.Exec("NOT VALID SQL")
	require.Error(t, err)
}
