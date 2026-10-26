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
