/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package _delete

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/query/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/query/cond"
)

// Query is the query state after DELETE FROM
type Query interface {
	// Where specifies the where clause
	Where(cond.Condition) whereQuery
}

// whereQuery is the query state after WHERE
type whereQuery interface {
	// Format composes the query and the params to pass to the DB
	Format(common.CondInterpreter) (string, []common.Param)

	// FormatTo composes the query and the params to pass to the DB with an offset for the numbered params
	FormatTo(common.CondInterpreter, common.Builder)
}
