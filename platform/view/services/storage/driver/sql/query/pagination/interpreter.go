/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pagination

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/query/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/query/cond"
	_select "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/query/select"
)

func NewDefaultInterpreter() *interpreter {
	return &interpreter{}
}

type interpreter struct{}

func handleKeysetPreProcess[T comparable](pagination *keyset[T, any], query common.ModifiableQuery) {
	query.AddField(pagination.SqlIdName)
	query.AddOrderBy(_select.Asc(pagination.SqlIdName))
	query.AddLimit(pagination.PageSize)
	if pagination.FirstId != pagination.nilElement() {
		query.AddWhere(cond.CmpVal(pagination.SqlIdName, ">", pagination.FirstId))
	} else {
		query.AddOffset(pagination.Offset)
	}
}

func (i *interpreter) PreProcess(p driver.Pagination, query common.ModifiableQuery) {
	switch pagination := p.(type) {
	case *none:
		return

	case *offset:
		query.AddLimit(pagination.PageSize)
		query.AddOffset(pagination.Offset)

	case *keyset[string, any]:
		handleKeysetPreProcess(pagination, query)

	case *keyset[int, any]:
		handleKeysetPreProcess(pagination, query)

	case *empty:
		query.AddLimit(0)

	default:
		panic(fmt.Sprintf("invalid pagination option %+v", pagination))
	}
}
