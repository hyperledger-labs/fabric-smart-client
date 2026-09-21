/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOpenSkipPragmas checks a database opens when the default pragmas are skipped, both
// with and without WAL mode set in the data source.
func TestOpenSkipPragmas(t *testing.T) {
	t.Parallel()

	t.Run("without WAL", func(t *testing.T) {
		t.Parallel()

		dbs, err := open(Opts{DataSource: tempDataSource(t, "nowal"), SkipPragmas: true})
		require.NoError(t, err)
		assert.NotNil(t, dbs)
	})

	t.Run("with WAL", func(t *testing.T) {
		t.Parallel()

		dbs, err := open(Opts{DataSource: tempDataSource(t, "wal") + "&_pragma=journal_mode(WAL)", SkipPragmas: true})
		require.NoError(t, err)
		assert.NotNil(t, dbs)
	})
}

// TestGetDirMalformedDataSource checks a data source that is not a parseable URL yields no
// directory rather than an error.
func TestGetDirMalformedDataSource(t *testing.T) {
	t.Parallel()

	assert.Empty(t, getDir("file:///tmp/%zz.sqlite"))
}
