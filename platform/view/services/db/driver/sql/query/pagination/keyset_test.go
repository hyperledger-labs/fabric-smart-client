/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pagination_test

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
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
		Format(nil, pagination.NewDefaultInterpreter())
	Expect(query).To(Equal("SELECT field1, col_id FROM test AS test ORDER BY col_id ASC LIMIT $1 OFFSET $2"))
	Expect(args).To(ConsistOf(10, 2))

	results := collections.NewSliceIterator([]*dbResult{
		{StringField: "first"},
		{StringField: "middle"},
		{StringField: "last"},
	})
	page, err := pagination.NewPage[dbResult](results, p)
	Expect(err).ToNot(HaveOccurred())

	query, args = q.Select().
		FieldsByName("field1").
		From(q.Table("test")).
		Paginated(page.Pagination).
		Format(nil, pagination.NewDefaultInterpreter())
	Expect(query).To(Equal("SELECT field1, col_id FROM test AS test WHERE (col_id > $1) ORDER BY col_id ASC LIMIT $2"))
	Expect(args).To(ConsistOf("last", 10))
}
