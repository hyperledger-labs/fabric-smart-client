/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package _delete

import (
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
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
	return q.FormatWithOffset(ci, common2.CopyPtr(1))
}

func (q *query) FormatWithOffset(ci common.CondInterpreter, pc *int) (string, []common.Param) {
	sb := common.NewBuilderWithOffset(pc).
		WriteString("DELETE FROM ").
		WriteString(string(q.table))

	if q.where != nil {
		sb.WriteString(" WHERE ").
			WriteConditionSerializable(q.where, ci)
	}

	return sb.Build()
}
