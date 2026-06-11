/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestCopyPtr(t *testing.T) {
	t.Parallel()

	t.Run("creates pointer copy", func(t *testing.T) {
		t.Parallel()
		original := 42
		ptr := CopyPtr(original)

		require.NotNil(t, ptr)
		require.Equal(t, original, *ptr)

		// Verify it's a copy
		*ptr = 100
		require.Equal(t, 42, original)
	})

	t.Run("handles different types", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "test", *CopyPtr("test"))
		require.Equal(t, 0, *CopyPtr(0))
		require.Equal(t, []int{1, 2}, *CopyPtr([]int{1, 2}))
	})
}

func TestClose(t *testing.T) {
	t.Parallel()

	t.Run("closes same DB once", func(t *testing.T) {
		t.Parallel()
		db, err := sql.Open("sqlite", ":memory:")
		require.NoError(t, err)

		require.NoError(t, Close(db, db))
		require.Error(t, db.Ping())
	})

	t.Run("closes different DBs", func(t *testing.T) {
		t.Parallel()
		readDB, err := sql.Open("sqlite", ":memory:")
		require.NoError(t, err)
		writeDB, err := sql.Open("sqlite", ":memory:")
		require.NoError(t, err)

		require.NoError(t, Close(readDB, writeDB))
		require.Error(t, readDB.Ping())
		require.Error(t, writeDB.Ping())
	})
}

func TestRWDB(t *testing.T) {
	t.Parallel()

	t.Run("same DB", func(t *testing.T) {
		t.Parallel()
		db, err := sql.Open("sqlite", ":memory:")
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, db.Close())
		})

		rwdb := &RWDB{ReadDB: db, WriteDB: db}
		require.Same(t, rwdb.ReadDB, rwdb.WriteDB)
	})

	t.Run("different DBs", func(t *testing.T) {
		t.Parallel()
		readDB, err := sql.Open("sqlite", ":memory:")
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, readDB.Close())
		})
		writeDB, err := sql.Open("sqlite", ":memory:")
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, writeDB.Close())
		})

		rwdb := &RWDB{ReadDB: readDB, WriteDB: writeDB}
		require.NotSame(t, rwdb.ReadDB, rwdb.WriteDB)
	})
}

func BenchmarkCopyPtr(b *testing.B) {
	b.Run("int", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = CopyPtr(42)
		}
	})

	b.Run("struct", func(b *testing.B) {
		type T struct {
			Name string
			Data []byte
		}
		v := T{Name: "test", Data: make([]byte, 100)}
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			_ = CopyPtr(v)
		}
	})
}
