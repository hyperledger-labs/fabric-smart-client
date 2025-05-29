/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package _delete

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/cond"
)

type query struct {
	table common.TableName
	where cond.Condition
}

func NewQuery() *query {
	return &query{}
}

func (q *query) From(t common.TableName) Query {
	q.table = t
	return q
}

func (q *query) Where(where cond.Condition) whereQuery {
	q.where = where
	return q
}

func (q *query) Format(ci common.CondInterpreter) (string, []common.Param) {
	sb := common.NewBuilder()
	q.FormatTo(ci, sb)
	return sb.Build()
}

func (q *query) FormatTo(ci common.CondInterpreter, sb common.Builder) {
	sb.WriteString("DELETE FROM ").
		WriteString(string(q.table))

	if q.where != nil && q.where != cond.AlwaysTrue {
		sb.WriteString(" WHERE ").
			WriteConditionSerializable(q.where, ci)
	}
}
