/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
)

type interpreter struct {
	common.Interpreter
}

func NewInterpreter() *interpreter {
	return &interpreter{Interpreter: common.NewInterpreter()}
}

func (i *interpreter) In(field common.FieldName, vals []any) common.Condition {
	tuples := make([]common.Tuple, len(vals))
	for i, val := range vals {
		tuples[i] = common.Tuple{val}
	}
	return i.InTuple([]common.FieldName{field}, tuples)
}
func (i *interpreter) InStrings(field common.FieldName, vals []string) common.Condition {
	tuples := make([]common.Tuple, len(vals))
	for i, val := range vals {
		tuples[i] = common.Tuple{val}
	}
	return i.InTuple([]common.FieldName{field}, tuples)
}
func (i *interpreter) InInts(field common.FieldName, vals []int) common.Condition {
	tuples := make([]common.Tuple, len(vals))
	for i, val := range vals {
		tuples[i] = common.Tuple{val}
	}
	return i.InTuple([]common.FieldName{field}, tuples)
}
func (i *interpreter) InTuple(fields []common.FieldName, vals []common.Tuple) common.Condition {
	if len(vals) == 0 || len(fields) == 0 {
		return common.EmptyCondition
	}
	ors := make([]common.Condition, len(vals))
	for j, tuple := range vals {
		ands := make([]common.Condition, len(tuple))
		for k, val := range tuple {
			ands[k] = i.Cmp(fields[k], "=", val)
		}
		ors[j] = i.And(ands...)
	}
	return i.Or(ors...)
}
