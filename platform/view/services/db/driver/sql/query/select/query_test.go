/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package _select_test

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/postgres"
	q "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/cond"
	. "github.com/onsi/gomega"
)

func TestSelectSimple(t *testing.T) {
	RegisterTestingT(t)

	myTable := q.Table("my_table")
	query, params := q.Select().FieldsByName("id", "name").
		From(myTable).
		Where(cond.CmpVal(myTable.Field("id"), ">", 5)).
		OrderBy(q.Asc(common.FieldName("id"))).
		Limit(2).
		Offset(1).
		Format(postgres.NewConditionInterpreter())

	Expect(query).To(Equal("SELECT id, name " +
		"FROM my_table " +
		"WHERE my_table.id > $1 " +
		"ORDER BY id ASC " +
		"LIMIT $2 " +
		"OFFSET $3"))
	Expect(params).To(ConsistOf(5, 2, 1))
}

func TestSelectJoin(t *testing.T) {
	RegisterTestingT(t)

	myTable, yourTable, theirTable := q.Table("my_table"), q.Table("your_table"), q.AliasedTable("their_table", "tt")
	query, params := q.Select().Fields(myTable.Field("name"), yourTable.Field("id")).
		From(myTable.
			Join(yourTable, cond.Cmp(myTable.Field("id"), "=", yourTable.Field("my_id"))).
			Join(theirTable, cond.Cmp(myTable.Field("id"), ">", theirTable.Field("their_id")))).
		Where(cond.CmpVal(myTable.Field("id"), ">", 5)).
		OrderBy(q.Desc(yourTable.Field("date"))).
		Limit(2).
		Offset(1).
		Format(postgres.NewConditionInterpreter())

	Expect(query).To(Equal("SELECT my_table.name, your_table.id " +
		"FROM my_table " +
		"LEFT JOIN your_table ON my_table.id = your_table.my_id " +
		"LEFT JOIN their_table AS tt ON my_table.id > tt.their_id " +
		"WHERE my_table.id > $1 " +
		"ORDER BY your_table.date DESC " +
		"LIMIT $2 " +
		"OFFSET $3"))
	Expect(params).To(ConsistOf(5, 2, 1))
}
