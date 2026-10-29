/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pagination_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/internal/storage/sqlbuild"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/pagination"
)

type dbResult struct {
	StringField        string
	IntField           int
	NonComparableField any
}

// render builds the query a caller would build for a paginated read of two
// columns, so the assertions below observe exactly what the pagination
// contributes: the WHERE cursor, ORDER BY, LIMIT and OFFSET.
func render(p driver.Pagination) (string, []sqlbuild.Param) {
	pag := pagination.Paging(p)
	return sqlbuild.New().
		WriteString("SELECT field1, col_id FROM test").
		WriteWhere(pag.Where).
		WritePaging(pag).
		Build()
}

func setupPaginationWithLastID(t *testing.T) *driver.PageIterator[*any] {
	t.Helper()
	p, err := pagination.KeysetWithField[string](200, 10, "col_id", "StringField")
	require.NoError(t, err)
	query, args := render(p)
	require.Equal(t, "SELECT field1, col_id FROM test ORDER BY col_id ASC LIMIT 10 OFFSET 200", query)
	require.Nil(t, args)

	results := collections.NewSliceIterator([]*any{
		common.CopyPtr[any](dbResult{StringField: "first"}),
		common.CopyPtr[any](dbResult{StringField: "2"}),
		common.CopyPtr[any](dbResult{StringField: "3"}),
		common.CopyPtr[any](dbResult{StringField: "4"}),
		common.CopyPtr[any](dbResult{StringField: "5"}),
		common.CopyPtr[any](dbResult{StringField: "6"}),
		common.CopyPtr[any](dbResult{StringField: "7"}),
		common.CopyPtr[any](dbResult{StringField: "8"}),
		common.CopyPtr[any](dbResult{StringField: "9"}),
		common.CopyPtr[any](dbResult{StringField: "last"}),
	})
	page, err := pagination.NewPage[any](results, p)
	require.NoError(t, err)

	return page
}

// Next() straight after a full page carries the last id forward as a cursor.
func TestKeysetSimple(t *testing.T) {
	t.Parallel()

	page := setupPaginationWithLastID(t)

	nextPagination, err := page.Pagination.Next()
	require.NoError(t, err)
	page.Pagination = nextPagination

	query, args := render(page.Pagination)
	require.Equal(t, "SELECT field1, col_id FROM test WHERE col_id > $1 ORDER BY col_id ASC LIMIT 10", query)
	require.Equal(t, []sqlbuild.Param{"last"}, args)
}

// Skipping a page loses the cursor, so it falls back to OFFSET.
func TestKeysetSkippingPage(t *testing.T) {
	t.Parallel()

	page := setupPaginationWithLastID(t)

	nextPagination, err := page.Pagination.Next()
	require.NoError(t, err)
	page.Pagination = nextPagination

	nextPagination, err = page.Pagination.Next()
	require.NoError(t, err)
	page.Pagination = nextPagination

	query, args := render(page.Pagination)
	require.Equal(t, "SELECT field1, col_id FROM test ORDER BY col_id ASC LIMIT 10 OFFSET 220", query)
	require.Nil(t, args)
}

func TestKeysetGoingBack(t *testing.T) {
	t.Parallel()

	page := setupPaginationWithLastID(t)

	nextPagination, err := page.Pagination.Prev()
	page.Pagination = nextPagination
	require.NoError(t, err)

	query, args := render(page.Pagination)
	require.Equal(t, "SELECT field1, col_id FROM test ORDER BY col_id ASC LIMIT 10 OFFSET 190", query)
	require.Nil(t, args)
}

func TestKeysetGoingNextBack(t *testing.T) {
	t.Parallel()

	page := setupPaginationWithLastID(t)

	nextPagination, err := page.Pagination.Next()
	page.Pagination = nextPagination
	require.NoError(t, err)

	nextPagination, err = page.Pagination.Next()
	page.Pagination = nextPagination
	require.NoError(t, err)

	nextPagination, err = page.Pagination.Prev()
	page.Pagination = nextPagination
	require.NoError(t, err)

	query, args := render(page.Pagination)
	require.Equal(t, "SELECT field1, col_id FROM test ORDER BY col_id ASC LIMIT 10 OFFSET 210", query)
	require.Nil(t, args)
}

// With no rows there is no last id, so the next page uses OFFSET, not a cursor.
func TestKeysetEmptyResults(t *testing.T) {
	t.Parallel()

	p, err := pagination.KeysetWithField[string](200, 10, "col_id", "StringField")
	require.NoError(t, err)
	query, args := render(p)
	require.Equal(t, "SELECT field1, col_id FROM test ORDER BY col_id ASC LIMIT 10 OFFSET 200", query)
	require.Nil(t, args)

	results := collections.NewSliceIterator([]*any{})
	page, err := pagination.NewPage[any](results, p)
	require.NoError(t, err)

	nextPagination, err := page.Pagination.Next()
	require.NoError(t, err)
	page.Pagination = nextPagination

	query, args = render(page.Pagination)
	require.Equal(t, "SELECT field1, col_id FROM test ORDER BY col_id ASC LIMIT 10 OFFSET 210", query)
	require.Nil(t, args)
}

func TestKeysetPartialResults(t *testing.T) {
	t.Parallel()

	p, err := pagination.KeysetWithField[string](200, 20, "col_id", "StringField")
	require.NoError(t, err)
	query, args := render(p)
	require.Equal(t, "SELECT field1, col_id FROM test ORDER BY col_id ASC LIMIT 20 OFFSET 200", query)
	require.Nil(t, args)

	results := collections.NewSliceIterator([]*any{})
	page, err := pagination.NewPage[any](results, p)
	require.NoError(t, err)

	nextPagination, err := page.Pagination.Next()
	require.NoError(t, err)
	page.Pagination = nextPagination

	query, args = render(page.Pagination)
	require.Equal(t, "SELECT field1, col_id FROM test ORDER BY col_id ASC LIMIT 20 OFFSET 220", query)
	require.Nil(t, args)
}

// An int id yields an int cursor parameter.
func TestKeysetInt(t *testing.T) {
	t.Parallel()

	p, err := pagination.KeysetWithField[int](200, 10, "col_id", "IntField")
	require.NoError(t, err)
	query, args := render(p)
	require.Equal(t, "SELECT field1, col_id FROM test ORDER BY col_id ASC LIMIT 10 OFFSET 200", query)
	require.Nil(t, args)

	results := collections.NewSliceIterator([]*any{
		common.CopyPtr[any](dbResult{IntField: 1}),
		common.CopyPtr[any](dbResult{IntField: 2}),
		common.CopyPtr[any](dbResult{IntField: 3}),
		common.CopyPtr[any](dbResult{IntField: 4}),
		common.CopyPtr[any](dbResult{IntField: 5}),
		common.CopyPtr[any](dbResult{IntField: 6}),
		common.CopyPtr[any](dbResult{IntField: 7}),
		common.CopyPtr[any](dbResult{IntField: 8}),
		common.CopyPtr[any](dbResult{IntField: 9}),
		common.CopyPtr[any](dbResult{IntField: 10}),
	})
	page, err := pagination.NewPage[any](results, p)
	require.NoError(t, err)

	nextPagination, err := page.Pagination.Next()
	require.NoError(t, err)
	page.Pagination = nextPagination

	query, args = render(page.Pagination)
	require.Equal(t, "SELECT field1, col_id FROM test WHERE col_id > $1 ORDER BY col_id ASC LIMIT 10", query)
	require.Equal(t, []sqlbuild.Param{10}, args)
}

func TestKeysetSerialization(t *testing.T) {
	t.Parallel()

	page := setupPaginationWithLastID(t)

	buf, err := page.Pagination.Serialize()
	require.NoError(t, err)

	k2, err := pagination.KeysetFromRaw[string](buf, "StringField")
	require.NoError(t, err)
	require.True(t, k2.Equal(page.Pagination))
}
