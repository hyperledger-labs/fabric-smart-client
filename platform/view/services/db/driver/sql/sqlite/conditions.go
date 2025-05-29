/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"math"
	"strconv"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/cond"
)

var signs = map[bool]rune{true: '+', false: '-'}

func NewConditionInterpreter() common.CondInterpreter {
	return &interpreter{}
}

type interpreter struct{}

func (i *interpreter) TimeOffset(duration time.Duration, sb common.Builder) {
	sb.WriteString("datetime('now'")
	if duration == 0 {
		sb.WriteRune(')')
		return
	}
	sb.WriteString(", '").
		WriteRune(signs[duration > 0]).
		WriteString(strconv.Itoa(int(math.Abs(duration.Seconds())))).
		WriteString(" seconds')")
}

func (i *interpreter) InTuple(fields []common.Serializable, vals []common.Tuple, sb common.Builder) {
	if len(vals) == 0 || len(fields) == 0 {
		return
	}
	if len(vals) == 1 && len(fields) == 1 {
		// Not necessary, but makes the query more readable
		sb.WriteConditionSerializable(cond.CmpVal(fields[0], "=", vals[0][0]), i)
		return
	}

	sb.WriteString("(").
		WriteSerializables(common.ToSerializables(fields)...).
		WriteString(") IN (").
		WriteTuples(vals).
		WriteString(")")
}
