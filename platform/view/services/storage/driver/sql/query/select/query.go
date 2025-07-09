/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package _select

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/query/common"
	cond2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/query/cond"
)

func NewQuery(distinct bool) *query {
	return &query{distinct: distinct}
}

type query struct {
	table      common2.JoinedTable
	distinct   bool
	fields     []common2.Field
	where      cond2.Condition
	limit      int
	offset     int
	orderBy    []OrderBy
	pagination driver.Pagination
}

func (q *query) AllFields() fieldsQuery {
	return q
}

func (q *query) FieldsByName(names ...common2.FieldName) fieldsQuery {
	fields := make([]common2.Field, len(names))
	for i, n := range names {
		fields[i] = n
	}
	return q.Fields(fields...)
}

func (q *query) Fields(fields ...common2.Field) fieldsQuery {
	q.fields = fields
	return q
}

func (q *query) From(t common2.JoinedTable) fromQuery {
	q.table = t
	return q
}

func (q *query) Limit(l int) limitQuery {
	q.limit = l
	return q
}

func (q *query) Offset(o int) offsetQuery {
	q.offset = o
	return q
}

func (q *query) OrderBy(os ...OrderBy) orderByQuery {
	q.orderBy = os
	return q
}

func (q *query) Where(p cond2.Condition) whereQuery {
	q.where = p
	return q
}

func (q *query) Paginated(p driver.Pagination) paginatedQuery {
	q.pagination = p
	return q
}

func (q *query) Format(ci common2.CondInterpreter) (string, []any) {
	return q.FormatPaginated(ci, nil)
}

func (q *query) FormatTo(ci common2.CondInterpreter, sb common2.Builder) {
	q.FormatPaginatedTo(ci, nil, sb)
}

func (q *query) FormatPaginated(ci common2.CondInterpreter, pi common2.PagInterpreter) (string, []any) {
	sb := common2.NewBuilder()
	q.FormatPaginatedTo(ci, pi, sb)
	return sb.Build()
}

func (q *query) FormatPaginatedTo(ci common2.CondInterpreter, pi common2.PagInterpreter, sb common2.Builder) {
	sb.WriteString("SELECT ")

	if q.distinct {
		sb.WriteString("DISTINCT ")
	}

	if len(q.fields) > 0 {
		sb.WriteSerializables(common2.ToSerializables(q.fields)...)
	} else {
		sb.WriteRune('*')
	}

	sb.WriteString(" FROM ").WriteConditionSerializable(q.table, ci)

	if q.where != nil && q.where != cond2.AlwaysTrue {
		sb.WriteString(" WHERE ").WriteConditionSerializable(q.where, ci)
	}

	if len(q.orderBy) > 0 {
		sb.WriteString(" ORDER BY ").WriteSerializables(common2.ToSerializables(q.orderBy)...)
	}

	if q.pagination != nil {
		pi.Interpret(q.pagination, sb)
		return
	}

	if q.limit > 0 {
		sb.WriteString(" LIMIT ").WriteParam(q.limit)
	}

	if q.offset > 0 {
		sb.WriteString(" OFFSET ").WriteParam(q.offset)
	}
}
