/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pagination_test

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	q "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/query"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/query/pagination"
	. "github.com/onsi/gomega"
)

func TestOffsetSimple(t *testing.T) {
	RegisterTestingT(t)

	query, args := q.Select().
		AllFields().
		From(q.Table("test")).
		Paginated(utils.MustGet(pagination.Offset(2, 10))).
		FormatPaginated(nil, pagination.NewDefaultInterpreter())

	Expect(query).To(Equal("SELECT * FROM test LIMIT $1 OFFSET $2"))
	Expect(args).To(ConsistOf(10, 2))
}
