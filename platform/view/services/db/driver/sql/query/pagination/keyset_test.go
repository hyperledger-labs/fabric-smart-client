/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pagination_test

import (
	"fmt"
	"testing"

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

func TestKeysetSimple(t *testing.T) {
	RegisterTestingT(t)

	p := utils.MustGet(pagination.KeysetWithField[string](2, 10, "col_id", "StringField"))
	query, args := q.Select().
		FieldsByName("field1").
		From(q.Table("test")).
		Paginated(p).
		FormatPaginated(nil, pagination.NewDefaultInterpreter())
	Expect(query).To(Equal("SELECT field1, col_id FROM test ORDER BY col_id ASC LIMIT $1 OFFSET $2"))
	Expect(args).To(ConsistOf(10, 2))

	results := collections.NewSliceIterator([]*any{
		common.CopyPtr[any](dbResult{StringField: "first"}),
		common.CopyPtr[any](dbResult{StringField: "middle"}),
		common.CopyPtr[any](dbResult{StringField: "last"}),
	})
	page, err := pagination.NewPage[any](results, p)
	Expect(err).ToNot(HaveOccurred())
	page.Pagination, err = page.Pagination.Next()
	Expect(err).ToNot(HaveOccurred())

	query, args = q.Select().
		FieldsByName("field1").
		From(q.Table("test")).
		Paginated(page.Pagination).
		FormatPaginated(nil, pagination.NewDefaultInterpreter())
	Expect(query).To(Equal("SELECT field1, col_id FROM test WHERE (col_id > $1) ORDER BY col_id ASC LIMIT $2"))
	Expect(args).To(ConsistOf("last", 10))

	// test that skipping a page resets lastId
	page.Pagination, err = page.Pagination.Next()
	Expect(err).ToNot(HaveOccurred())
	query, args = q.Select().
		FieldsByName("field1").
		From(q.Table("test")).
		Paginated(p).
		FormatPaginated(nil, pagination.NewDefaultInterpreter())
	fmt.Printf("query = %s\n", query)
	Expect(query).To(Equal("SELECT field1, col_id FROM test ORDER BY col_id ASC LIMIT $1 OFFSET $2"))
	Expect(args).To(ConsistOf(10, 22))
}
