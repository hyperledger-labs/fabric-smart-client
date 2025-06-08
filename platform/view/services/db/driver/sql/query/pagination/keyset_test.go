/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pagination_test

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	q "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/pagination"
	. "github.com/onsi/gomega"
)

type dbResult struct {
	StringField        string
	IntField           int
	NonComparableField any
	unexportedField    string
}

func setupPaginationWithLastId() *driver.PageIterator[*interface{}] {
	p := utils.MustGet(pagination.KeysetWithField[string](200, 10, "col_id", "StringField"))
	query, args := q.Select().
		FieldsByName("field1").
		From(q.Table("test")).
		Paginated(p).
		FormatPaginated(nil, pagination.NewDefaultInterpreter())
	Expect(query).To(Equal("SELECT field1, col_id FROM test ORDER BY col_id ASC LIMIT $1 OFFSET $2"))
	Expect(args).To(ConsistOf(10, 200))

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

func TestKeysetSimple(t *testing.T) {
	RegisterTestingT(t)

	page := setupPaginationWithLastId()

	nextPagination, err := page.Pagination.Next()
	Expect(err).ToNot(HaveOccurred())
	page.Pagination = nextPagination

	query, args := q.Select().
		FieldsByName("field1").
		From(q.Table("test")).
		Paginated(page.Pagination).
		FormatPaginated(nil, pagination.NewDefaultInterpreter())
	Expect(query).To(Equal("SELECT field1, col_id FROM test WHERE (col_id > $1) ORDER BY col_id ASC LIMIT $2"))
	Expect(args).To(ConsistOf("last", 10))
}

func TestKeysetSkippingPage(t *testing.T) {
	RegisterTestingT(t)

	page := setupPaginationWithLastId()

	nextPagination, err := page.Pagination.Next()
	Expect(err).ToNot(HaveOccurred())
	page.Pagination = nextPagination

	nextPagination, err = page.Pagination.Next()
	Expect(err).ToNot(HaveOccurred())
	page.Pagination = nextPagination

	query, args := q.Select().
		FieldsByName("field1").
		From(q.Table("test")).
		Paginated(page.Pagination).
		FormatPaginated(nil, pagination.NewDefaultInterpreter())
	Expect(query).To(Equal("SELECT field1, col_id FROM test ORDER BY col_id ASC LIMIT $1 OFFSET $2"))
	Expect(args).To(ConsistOf(10, 220))
}

func TestKeysetGoingBack(t *testing.T) {
	RegisterTestingT(t)

	page := setupPaginationWithLastId()

	nextPagination, err := page.Pagination.Prev()
	page.Pagination = nextPagination
	Expect(err).ToNot(HaveOccurred())

	query, args := q.Select().
		FieldsByName("field1").
		From(q.Table("test")).
		Paginated(page.Pagination).
		FormatPaginated(nil, pagination.NewDefaultInterpreter())
	Expect(query).To(Equal("SELECT field1, col_id FROM test ORDER BY col_id ASC LIMIT $1 OFFSET $2"))
	Expect(args).To(ConsistOf(10, 190))
}

func TestKeysetGoingNextBack(t *testing.T) {
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

	query, args := q.Select().
		FieldsByName("field1").
		From(q.Table("test")).
		Paginated(page.Pagination).
		FormatPaginated(nil, pagination.NewDefaultInterpreter())
	Expect(query).To(Equal("SELECT field1, col_id FROM test ORDER BY col_id ASC LIMIT $1 OFFSET $2"))
	Expect(args).To(ConsistOf(10, 210))
}

func TestKeysetEmptyResults(t *testing.T) {
	RegisterTestingT(t)

	p := utils.MustGet(pagination.KeysetWithField[string](200, 10, "col_id", "StringField"))
	query, args := q.Select().
		FieldsByName("field1").
		From(q.Table("test")).
		Paginated(p).
		FormatPaginated(nil, pagination.NewDefaultInterpreter())
	Expect(query).To(Equal("SELECT field1, col_id FROM test ORDER BY col_id ASC LIMIT $1 OFFSET $2"))
	Expect(args).To(ConsistOf(10, 200))

	results := collections.NewSliceIterator([]*any{})
	page, err := pagination.NewPage[any](results, p)
	Expect(err).ToNot(HaveOccurred())

	nextPagination, err := page.Pagination.Next()
	Expect(err).ToNot(HaveOccurred())
	page.Pagination = nextPagination

	query, args = q.Select().
		FieldsByName("field1").
		From(q.Table("test")).
		Paginated(page.Pagination).
		FormatPaginated(nil, pagination.NewDefaultInterpreter())
	Expect(query).To(Equal("SELECT field1, col_id FROM test ORDER BY col_id ASC LIMIT $1 OFFSET $2"))
	Expect(args).To(ConsistOf(10, 210))
}

func TestKeysetPartialResults(t *testing.T) {
	RegisterTestingT(t)

	p := utils.MustGet(pagination.KeysetWithField[string](200, 20, "col_id", "StringField"))
	query, args := q.Select().
		FieldsByName("field1").
		From(q.Table("test")).
		Paginated(p).
		FormatPaginated(nil, pagination.NewDefaultInterpreter())
	Expect(query).To(Equal("SELECT field1, col_id FROM test ORDER BY col_id ASC LIMIT $1 OFFSET $2"))
	Expect(args).To(ConsistOf(20, 200))

	results := collections.NewSliceIterator([]*any{})
	page, err := pagination.NewPage[any](results, p)
	Expect(err).ToNot(HaveOccurred())

	nextPagination, err := page.Pagination.Next()
	Expect(err).ToNot(HaveOccurred())
	page.Pagination = nextPagination

	query, args = q.Select().
		FieldsByName("field1").
		From(q.Table("test")).
		Paginated(page.Pagination).
		FormatPaginated(nil, pagination.NewDefaultInterpreter())
	Expect(query).To(Equal("SELECT field1, col_id FROM test ORDER BY col_id ASC LIMIT $1 OFFSET $2"))
	Expect(args).To(ConsistOf(20, 220))
}
