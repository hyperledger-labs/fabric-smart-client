/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package _select

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/cond"
)

func NewQuery(distinct bool) *query {
	return &query{distinct: distinct}
}

type query struct {
	table      common.JoinedTable
	distinct   bool
	fields     []common.Field
	where      cond.Condition
	limit      int
	offset     int
	orderBy    []OrderBy
	pagination driver.Pagination
}

func (q *query) AllFields() fieldsQuery {
	return q
}

func (q *query) FieldsByName(names ...common.FieldName) fieldsQuery {
	fields := make([]common.Field, len(names))
	for i, n := range names {
		fields[i] = n
	}
	return q.Fields(fields...)
}

func (q *query) Fields(fields ...common.Field) fieldsQuery {
	q.fields = fields
	return q
}

func (q *query) From(t common.JoinedTable) fromQuery {
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

func (q *query) Where(p cond.Condition) whereQuery {
	q.where = p
	return q
}

func (q *query) Paginated(p driver.Pagination) paginatedQuery {
	q.pagination = p
	return q
}

func (q *query) Format(ci common.CondInterpreter, pi common.PagInterpreter) (string, []any) {
	return q.FormatWithOffset(ci, pi, common2.CopyPtr(1))
}

func (q *query) FormatWithOffset(ci common.CondInterpreter, pi common.PagInterpreter, pc *int) (string, []any) {
	sb := common.NewBuilderWithOffset(pc).WriteString("SELECT ")

	if q.distinct {
		sb.WriteString("DISTINCT ")
	}

	if len(q.fields) > 0 {
		sb.WriteSerializables(common.ToSerializables(q.fields)...)
	} else {
		sb.WriteRune('*')
	}

	sb.WriteString(" FROM ").WriteConditionSerializable(q.table, ci)

	if q.where != nil && q.where != cond.AlwaysTrue {
		sb.WriteString(" WHERE ").WriteConditionSerializable(q.where, ci)
	}

	if len(q.orderBy) > 0 {
		sb.WriteString(" ORDER BY ").WriteSerializables(common.ToSerializables(q.orderBy)...)
	}

	if q.pagination != nil {
		pi.Interpret(q.pagination, sb)
		return sb.Build()
	}

	if q.limit > 0 {
		sb.WriteString(" LIMIT ").WriteParam(q.limit)
	}

	if q.offset > 0 {
		sb.WriteString(" OFFSET ").WriteParam(q.offset)
	}

	return sb.Build()
}
