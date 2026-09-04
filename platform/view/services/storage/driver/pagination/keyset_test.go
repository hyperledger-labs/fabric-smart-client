/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pagination_test

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/internal/storage/sqlbuild"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
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

func setupPaginationWithLastId() *driver.PageIterator[*any] {
	p := utils.MustGet(pagination.KeysetWithField[string](200, 10, "col_id", "StringField"))
	query, args := render(p)
	Expect(query).To(Equal("SELECT field1, col_id FROM test ORDER BY col_id ASC LIMIT 10 OFFSET 200"))
	Expect(args).To(BeNil())

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
	Expect(err).ToNot(HaveOccurred())

	return page
}

// Next() straight after a full page carries the last id forward as a cursor.
func TestKeysetSimple(t *testing.T) { //nolint:paralleltest
	RegisterTestingT(t)

	page := setupPaginationWithLastId()

	nextPagination, err := page.Pagination.Next()
	Expect(err).ToNot(HaveOccurred())
	page.Pagination = nextPagination

	query, args := render(page.Pagination)
	Expect(query).To(Equal("SELECT field1, col_id FROM test WHERE col_id > $1 ORDER BY col_id ASC LIMIT 10"))
	Expect(args).To(Equal([]sqlbuild.Param{"last"}))
}

// Skipping a page loses the cursor, so it falls back to OFFSET.
func TestKeysetSkippingPage(t *testing.T) { //nolint:paralleltest
	RegisterTestingT(t)

	page := setupPaginationWithLastId()

	nextPagination, err := page.Pagination.Next()
	Expect(err).ToNot(HaveOccurred())
	page.Pagination = nextPagination

	nextPagination, err = page.Pagination.Next()
	Expect(err).ToNot(HaveOccurred())
	page.Pagination = nextPagination

	query, args := render(page.Pagination)
	Expect(query).To(Equal("SELECT field1, col_id FROM test ORDER BY col_id ASC LIMIT 10 OFFSET 220"))
	Expect(args).To(BeNil())
}

func TestKeysetGoingBack(t *testing.T) { //nolint:paralleltest
	RegisterTestingT(t)

	page := setupPaginationWithLastId()

	nextPagination, err := page.Pagination.Prev()
	page.Pagination = nextPagination
	Expect(err).ToNot(HaveOccurred())

	query, args := render(page.Pagination)
	Expect(query).To(Equal("SELECT field1, col_id FROM test ORDER BY col_id ASC LIMIT 10 OFFSET 190"))
	Expect(args).To(BeNil())
}

func TestKeysetGoingNextBack(t *testing.T) { //nolint:paralleltest
	RegisterTestingT(t)

	page := setupPaginationWithLastId()

	nextPagination, err := page.Pagination.Next()
	page.Pagination = nextPagination
	Expect(err).ToNot(HaveOccurred())

	nextPagination, err = page.Pagination.Next()
	page.Pagination = nextPagination
	Expect(err).ToNot(HaveOccurred())

	nextPagination, err = page.Pagination.Prev()
	page.Pagination = nextPagination
	Expect(err).ToNot(HaveOccurred())

	query, args := render(page.Pagination)
	Expect(query).To(Equal("SELECT field1, col_id FROM test ORDER BY col_id ASC LIMIT 10 OFFSET 210"))
	Expect(args).To(BeNil())
}

// With no rows there is no last id, so the next page uses OFFSET, not a cursor.
func TestKeysetEmptyResults(t *testing.T) { //nolint:paralleltest
	RegisterTestingT(t)

	p := utils.MustGet(pagination.KeysetWithField[string](200, 10, "col_id", "StringField"))
	query, args := render(p)
	Expect(query).To(Equal("SELECT field1, col_id FROM test ORDER BY col_id ASC LIMIT 10 OFFSET 200"))
	Expect(args).To(BeNil())

	results := collections.NewSliceIterator([]*any{})
	page, err := pagination.NewPage[any](results, p)
	Expect(err).ToNot(HaveOccurred())

	nextPagination, err := page.Pagination.Next()
	Expect(err).ToNot(HaveOccurred())
	page.Pagination = nextPagination

	query, args = render(page.Pagination)
	Expect(query).To(Equal("SELECT field1, col_id FROM test ORDER BY col_id ASC LIMIT 10 OFFSET 210"))
	Expect(args).To(BeNil())
}

func TestKeysetPartialResults(t *testing.T) { //nolint:paralleltest
	RegisterTestingT(t)

	p := utils.MustGet(pagination.KeysetWithField[string](200, 20, "col_id", "StringField"))
	query, args := render(p)
	Expect(query).To(Equal("SELECT field1, col_id FROM test ORDER BY col_id ASC LIMIT 20 OFFSET 200"))
	Expect(args).To(BeNil())

	results := collections.NewSliceIterator([]*any{})
	page, err := pagination.NewPage[any](results, p)
	Expect(err).ToNot(HaveOccurred())

	nextPagination, err := page.Pagination.Next()
	Expect(err).ToNot(HaveOccurred())
	page.Pagination = nextPagination

	query, args = render(page.Pagination)
	Expect(query).To(Equal("SELECT field1, col_id FROM test ORDER BY col_id ASC LIMIT 20 OFFSET 220"))
	Expect(args).To(BeNil())
}

// An int id yields an int cursor parameter.
func TestKeysetInt(t *testing.T) { //nolint:paralleltest
	RegisterTestingT(t)

	p := utils.MustGet(pagination.KeysetWithField[int](200, 10, "col_id", "IntField"))
	query, args := render(p)
	Expect(query).To(Equal("SELECT field1, col_id FROM test ORDER BY col_id ASC LIMIT 10 OFFSET 200"))
	Expect(args).To(BeNil())

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
	Expect(err).ToNot(HaveOccurred())

	nextPagination, err := page.Pagination.Next()
	Expect(err).ToNot(HaveOccurred())
	page.Pagination = nextPagination

	query, args = render(page.Pagination)
	Expect(query).To(Equal("SELECT field1, col_id FROM test WHERE col_id > $1 ORDER BY col_id ASC LIMIT 10"))
	Expect(args).To(Equal([]sqlbuild.Param{10}))
}

func TestKeysetSeriliazation(t *testing.T) { //nolint:paralleltest
	RegisterTestingT(t)

	page := setupPaginationWithLastId()

	buf, err := page.Pagination.Serialize()
	Expect(err).ToNot(HaveOccurred())

	k2, err := pagination.KeysetFromRaw[string](buf, "StringField")
	Expect(err).ToNot(HaveOccurred())
	Expect(k2.Equal(page.Pagination)).To(Equal(true))
}
