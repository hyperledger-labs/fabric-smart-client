/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package _update

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/cond"
)

type set struct {
	field common.FieldName
	value common.Param
}

func (s set) WriteString(sb common.Builder) {
	sb.WriteSerializables(s.field).WriteString(" = ").WriteParam(s.value)
}

type query struct {
	table common.TableName
	sets  []set
	where cond.Condition
}

func NewQuery() *query {
	return &query{}
}

func (q *query) Update(t common.TableName) Query {
	q.table = t
	return q
}

func (q *query) Set(field common.FieldName, value common.Param) setQuery {
	q.sets = append(q.sets, set{field: field, value: value})
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
	sb.WriteString("UPDATE ").
		WriteString(string(q.table)).
		WriteString(" SET ").
		WriteSerializables(common.ToSerializables(q.sets)...)

	if q.where != nil && q.where != cond.AlwaysTrue {
		sb.WriteString(" WHERE ").WriteConditionSerializable(q.where, ci)
	}
}
