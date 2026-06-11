/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeTransaction struct {
	commitErr   error
	rollbackErr error
	committed   bool
	rolledBack  bool
}

func (f *fakeTransaction) Commit() error {
	f.committed = true
	return f.commitErr
}

func (f *fakeTransaction) Rollback() error {
	f.rolledBack = true
	return f.rollbackErr
}

func TestBaseDB_TransactionLifecycle(t *testing.T) {
	t.Parallel()

	newTxn := func() (*fakeTransaction, error) {
		return &fakeTransaction{}, nil
	}

	t.Run("begin and commit", func(t *testing.T) {
		t.Parallel()
		db := NewBaseDB(newTxn)

		require.True(t, db.IsTxnNil())

		err := db.BeginUpdate()
		require.NoError(t, err)
		require.False(t, db.IsTxnNil())

		err = db.Commit()
		require.NoError(t, err)
		require.True(t, db.IsTxnNil())
	})

	t.Run("begin and discard", func(t *testing.T) {
		t.Parallel()
		db := NewBaseDB(newTxn)

		err := db.BeginUpdate()
		require.NoError(t, err)

		err = db.Discard()
		require.NoError(t, err)
		require.True(t, db.IsTxnNil())
	})

	t.Run("multiple sequential transactions", func(t *testing.T) {
		t.Parallel()
		db := NewBaseDB(newTxn)

		for i := range 3 {
			err := db.BeginUpdate()
			require.NoError(t, err)

			if i%2 == 0 {
				err = db.Commit()
			} else {
				err = db.Discard()
			}
			require.NoError(t, err)
		}
	})
}

func TestBaseDB_Errors(t *testing.T) {
	t.Parallel()

	t.Run("begin fails", func(t *testing.T) {
		t.Parallel()
		expectedErr := errors.New("creation failed")
		newTxn := func() (*fakeTransaction, error) {
			return nil, expectedErr
		}
		db := NewBaseDB(newTxn)

		err := db.BeginUpdate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "error starting db transaction")
	})

	t.Run("commit fails", func(t *testing.T) {
		t.Parallel()
		expectedErr := errors.New("commit failed")
		newTxn := func() (*fakeTransaction, error) {
			return &fakeTransaction{commitErr: expectedErr}, nil
		}
		db := NewBaseDB(newTxn)

		err := db.BeginUpdate()
		require.NoError(t, err)

		err = db.Commit()
		require.Error(t, err)
		require.Contains(t, err.Error(), "could not commit transaction")
		require.True(t, db.IsTxnNil()) // State cleaned up even on error
	})

	t.Run("discard without begin", func(t *testing.T) {
		t.Parallel()
		db := NewBaseDB(func() (*fakeTransaction, error) {
			return &fakeTransaction{}, nil
		})

		err := db.Discard()
		require.Error(t, err)
		require.Contains(t, err.Error(), "no commit in progress")
	})

	t.Run("discard ignores rollback errors", func(t *testing.T) {
		t.Parallel()
		newTxn := func() (*fakeTransaction, error) {
			return &fakeTransaction{rollbackErr: errors.New("rollback failed")}, nil
		}
		db := NewBaseDB(newTxn)

		err := db.BeginUpdate()
		require.NoError(t, err)

		err = db.Discard()
		require.NoError(t, err) // Error is ignored
	})
}

func TestBaseDB_StateManagement(t *testing.T) {
	t.Parallel()

	newTxn := func() (*fakeTransaction, error) {
		return &fakeTransaction{}, nil
	}

	t.Run("commit cleans up state", func(t *testing.T) {
		t.Parallel()
		db := NewBaseDB(newTxn)

		err := db.BeginUpdate()
		require.NoError(t, err)
		require.False(t, db.IsTxnNil())

		err = db.Commit()
		require.NoError(t, err)
		require.True(t, db.IsTxnNil())

		// Can start new transaction
		err = db.BeginUpdate()
		require.NoError(t, err)
		_ = db.Discard()
	})

	t.Run("discard cleans up state", func(t *testing.T) {
		t.Parallel()
		db := NewBaseDB(newTxn)

		err := db.BeginUpdate()
		require.NoError(t, err)

		err = db.Discard()
		require.NoError(t, err)
		require.True(t, db.IsTxnNil())

		// Can start new transaction
		err = db.BeginUpdate()
		require.NoError(t, err)
		_ = db.Discard()
	})

	t.Run("commit and discard are mutually exclusive", func(t *testing.T) {
		t.Parallel()
		db := NewBaseDB(newTxn)

		err := db.BeginUpdate()
		require.NoError(t, err)

		err = db.Commit()
		require.NoError(t, err)

		// Discard after commit fails
		err = db.Discard()
		require.Error(t, err)
	})
}

func BenchmarkBaseDB_BeginUpdate(b *testing.B) {
	newTxn := func() (*fakeTransaction, error) {
		return &fakeTransaction{}, nil
	}
	db := NewBaseDB(newTxn)

	b.ReportAllocs()

	for b.Loop() {
		_ = db.BeginUpdate()
		_ = db.Discard()
	}
}

func BenchmarkBaseDB_Commit(b *testing.B) {
	newTxn := func() (*fakeTransaction, error) {
		return &fakeTransaction{}, nil
	}
	db := NewBaseDB(newTxn)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = db.BeginUpdate()
		_ = db.Commit()
	}
}

func BenchmarkBaseDB_Discard(b *testing.B) {
	newTxn := func() (*fakeTransaction, error) {
		return &fakeTransaction{}, nil
	}
	db := NewBaseDB(newTxn)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = db.BeginUpdate()
		_ = db.Discard()
	}
}
