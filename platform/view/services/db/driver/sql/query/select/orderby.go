/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package _select

import "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/common"

type OrderBy = common.OrderBy

type orderBy struct {
	asc   bool
	field common.Field
}

func (o orderBy) WriteString(sb common.Builder) {
	sb.WriteSerializables(o.field)

	if o.asc {
		sb.WriteString(" ASC")
	} else {
		sb.WriteString(" DESC")
	}
}

func Asc(name common.Field) orderBy {
	return orderBy{asc: true, field: name}
}

func Desc(name common.Field) orderBy {
	return orderBy{asc: false, field: name}
}
