/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pagination

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/query/common"
)

func NewDefaultInterpreter() *interpreter {
	return &interpreter{}
}

type Interpreter = common.PagInterpreter

type interpreter struct{}

func (i *interpreter) Interpret(p driver.Pagination, sb common.Builder) {
	switch pagination := p.(type) {
	case *none:
		return
	case *offset:
		sb.WriteString(" LIMIT ").
			WriteParam(pagination.pageSize).
			WriteString(" OFFSET ").
			WriteParam(pagination.offset)
	case *empty:
		sb.WriteString(" LIMIT 0 OFFSET 0")
		return
	default:
		panic(fmt.Sprintf("invalid pagination option %+v", pagination))
	}
}
