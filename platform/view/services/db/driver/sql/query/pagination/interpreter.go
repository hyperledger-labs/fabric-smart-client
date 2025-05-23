/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pagination

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/cond"
	_select "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/select"
)

func NewDefaultInterpreter() *interpreter {
	return &interpreter{}
}

type interpreter struct{}

func (i *interpreter) PreProcess(p driver.Pagination, query common.ModifiableQuery) {
	switch pagination := p.(type) {
	case *none:
		query.AddLimit(0)
		query.AddOffset(0)
		return
	case *offset:

		query.AddLimit(pagination.pageSize)
		query.AddOffset(pagination.offset)

	case *keyset[string, any]:
		query.AddField(pagination.sqlIdName)
		query.AddOrderBy(_select.Asc(pagination.sqlIdName))
		query.AddLimit(pagination.pageSize)
		if len(pagination.lastId) > 0 {
			query.AddWhere(cond.CmpVal(pagination.sqlIdName, ">", pagination.lastId))
		} else {
			query.AddOffset(pagination.offset)
		}
	case *empty:
		query.AddLimit(0)
		query.AddOffset(0)
	default:
		panic(fmt.Sprintf("invalid pagination option %+v", pagination))
	}
}
