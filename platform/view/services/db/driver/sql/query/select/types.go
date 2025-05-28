/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package _select

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/cond"
)

type Query interface {
	// AllFields selects all fields
	AllFields() fieldsQuery
	// Fields selects fully-qualified fields (table name and column)
	// Useful in case of conflicting names with joined tables
	Fields(...common.Field) fieldsQuery
	// FieldsByName selects fields only with their name
	// More handy for most cases
	FieldsByName(names ...common.FieldName) fieldsQuery
}

// Query is the query state after SELECT
type fieldsQuery interface {
	// From specifies a table (possibly with Joins)
	From(common.JoinedTable) fromQuery
}

// fromQuery is the query state after FROM
type fromQuery interface {
	whereQuery

	// Where specifies the where clause
	Where(cond.Condition) whereQuery
}

// whereQuery is the query state after WHERE
type whereQuery interface {
	orderByQuery

	// OrderBy specifies the order by clause
	OrderBy(...OrderBy) orderByQuery

	// Paginated specifies the pagination details
	Paginated(driver.Pagination) paginatedQuery
}

type paginatedQuery interface {
	FormatPaginated(common.CondInterpreter, common.PagInterpreter) (string, []common.Param)

	FormatPaginatedWithOffset(ci common.CondInterpreter, pi common.PagInterpreter, pc *int) (string, []any)
}

// orderByQuery is the query state after ORDER BY
type orderByQuery interface {
	limitQuery
	offsetQuery

	// Limit specifies the limit
	Limit(int) limitQuery
}

// limitQuery is the query state after LIMIT
type limitQuery interface {
	// Offset specifies the offset
	offsetQuery
	Offset(int) offsetQuery
}

// offsetQuery is the query state after OFFSET
type offsetQuery interface {
	// Format composes the query and the params to pass to the DB
	Format(common.CondInterpreter) (string, []common.Param)

	// FormatWithOffset composes the query and the params to pass to the DB with an offset for the numbered params
	FormatWithOffset(common.CondInterpreter, *int) (string, []common.Param)
}
