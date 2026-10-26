/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common_test

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/mock"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
)

// TestIteratorReturnsRowError checks that an error raised while reading rows,
// such as a connection lost mid-result, is returned by Next instead of ending
// the iteration as if the result set were complete.
func TestIteratorReturnsRowError(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	readErr := errors.New("connection reset by peer")
	mock.ExpectQuery("SELECT").WillReturnRows(
		sqlmock.NewRows([]string{"v"}).AddRow("a").AddRow("b").RowError(1, readErr))

	rows, err := db.QueryContext(t.Context(), "SELECT v FROM t")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	it := common2.NewIterator(rows, func(v *string) error { return rows.Scan(v) })

	v, err := it.Next()
	require.NoError(t, err)
	require.Equal(t, "a", *v)

	v, err = it.Next()
	require.ErrorIs(t, err, readErr)
	require.Nil(t, v)
}

// TestWrappedIteratorWrapsErrors checks that NewWrappedIterator passes row-read
// and scan errors through the SQLErrorWrapper, so callers can match the storage
// driver sentinels.
func TestWrappedIteratorWrapsErrors(t *testing.T) {
	t.Parallel()

	readErr := errors.New("connection reset by peer")
	scanErr := errors.New("bad column")
	wrapper := &mock.SQLErrorWrapper{}
	wrapper.WrapErrorStub = func(err error) error { return errors.Wrap(driver.ErrConnectionLost, err.Error()) }

	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	sqlMock.ExpectQuery("SELECT").WillReturnRows(
		sqlmock.NewRows([]string{"v"}).AddRow("a").AddRow("b").RowError(1, readErr))

	rows, err := db.QueryContext(t.Context(), "SELECT v FROM t")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	first := true
	it := common2.NewWrappedIterator(rows, func(v *string) error {
		if first {
			first = false
			return scanErr
		}
		return rows.Scan(v)
	}, wrapper)

	_, err = it.Next()
	require.ErrorIs(t, err, driver.ErrConnectionLost)
	require.ErrorContains(t, err, scanErr.Error())

	v, err := it.Next()
	require.ErrorIs(t, err, driver.ErrConnectionLost)
	require.ErrorContains(t, err, readErr.Error())
	require.Nil(t, v)
	require.Equal(t, 2, wrapper.WrapErrorCallCount())
}
