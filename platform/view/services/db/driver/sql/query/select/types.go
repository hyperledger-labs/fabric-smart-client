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

// Query is the query state after SELECT
type Query interface {
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
	offsetQuery

	// Paginated specifies the pagination details
	Paginated(driver.Pagination) offsetQuery

	// OrderBy specifies the order by clause
	OrderBy(...OrderBy) orderByQuery
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
	Offset(int) offsetQuery
}

// offsetQuery is the query state after OFFSET
type offsetQuery interface {
	// Format composes the query and the params to pass to the DB
	Format(common.CondInterpreter, common.PagInterpreter) (string, []common.Param)

	// FormatWithOffset composes the query and the params to pass to the DB with an offset for the numbered params
	FormatWithOffset(common.CondInterpreter, common.PagInterpreter, *int) (string, []common.Param)
}
