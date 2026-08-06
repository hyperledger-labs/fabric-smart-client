/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/
package _select_test

import (
	"math"
	"strconv"
	"time"

	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/query/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/query/cond"
)

var signs = map[bool]rune{true: '+', false: '-'}

type testInterpreter struct{}

func newTestInterpreter() common2.CondInterpreter {
	return &testInterpreter{}
}

func (i *testInterpreter) TimeOffset(duration time.Duration, sb common2.Builder) {
	sb.WriteString("NOW()")
	if duration == 0 {
		return
	}
	sb.WriteRune(' ').
		WriteRune(signs[duration > 0]).
		WriteString(" INTERVAL '").
		WriteString(strconv.Itoa(int(math.Abs(duration.Seconds())))).
		WriteString(" seconds'")
}

func (i *testInterpreter) InTuple(fields []common2.Serializable, vals []common2.Tuple, sb common2.Builder) {
	if len(vals) == 0 || len(fields) == 0 {
		return
	}
	ors := make([]cond.Condition, len(vals))
	for j, tuple := range vals {
		ands := make([]cond.Condition, len(tuple))
		for k, val := range tuple {
			ands[k] = cond.CmpVal(fields[k], "=", val)
		}
		ors[j] = cond.And(ands...)
	}
	sb.WriteConditionSerializable(cond.Or(ors...), i)
}
