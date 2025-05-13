/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cond

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/common"
)

const (
	NoStringLimit = ""
	NoIntLimit    = -1
)

type Operator = string

type cmp struct {
	f1, f2 common.Serializable
	op     Operator
}

func (c *cmp) WriteString(_ common.CondInterpreter, sb common.Builder) {
	sb.WriteSerializables(c.f1).
		WriteRune(' ').
		WriteString(c.op).
		WriteRune(' ').
		WriteSerializables(c.f2)
}

func Cmp(f1 common.Serializable, op Operator, f2 common.Serializable) *cmp {
	return &cmp{f1, f2, op}
}

type cmpVal struct {
	f   common.Serializable
	op  Operator
	val common.Param
}

func (c *cmpVal) WriteString(_ common.CondInterpreter, sb common.Builder) {
	sb.WriteSerializables(c.f).
		WriteRune(' ').
		WriteString(c.op).
		WriteRune(' ').
		WriteParam(c.val)
}

func Eq(f common.FieldName, val common.Param) *cmpVal {
	return CmpVal(f, "=", val)
}

func CmpVal(f1 common.Serializable, op Operator, val common.Param) *cmpVal {
	return &cmpVal{f1, op, val}
}

type between[T comparable] struct {
	f                common.Serializable
	start, end, none T
}

func BetweenInts(f common.Serializable, start, end int) *between[int] {
	return &between[int]{f: f, start: start, end: end, none: NoIntLimit}
}

func BetweenStrings(f common.Serializable, start, end string) *between[string] {
	return &between[string]{f: f, start: start, end: end, none: NoStringLimit}
}

func (c *between[T]) WriteString(in common.CondInterpreter, sb common.Builder) {
	var conds []Condition
	if c.start != c.none {
		conds = append(conds, CmpVal(c.f, ">=", c.start))
	}
	if c.end != c.none {
		conds = append(conds, CmpVal(c.f, "<", c.end))
	}
	sb.WriteConditionSerializable(And(conds...), in)
}
