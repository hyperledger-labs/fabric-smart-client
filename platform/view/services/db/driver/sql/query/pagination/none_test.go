/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pagination_test

import (
	"testing"

	q "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/pagination"
	. "github.com/onsi/gomega"
)

func TestNone(t *testing.T) {
	RegisterTestingT(t)

	query, args := q.Select().
		AllFields().
		From(q.Table("test")).
		Paginated(pagination.None()).
		FormatPaginated(nil, pagination.NewDefaultInterpreter())

	Expect(query).To(Equal("SELECT * FROM test AS test"))
	Expect(args).To(BeEmpty())
}

func TestEmpty(t *testing.T) {
	RegisterTestingT(t)

	query, args := q.Select().
		AllFields().
		From(q.Table("test")).
		Paginated(pagination.Empty()).
		FormatPaginated(nil, pagination.NewDefaultInterpreter())

	Expect(query).To(Equal("SELECT * FROM test AS test"))
	Expect(args).To(BeEmpty())
}
