/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cond

import (
	"time"

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

func CmpVal(f1 common.Serializable, op Operator, val common.Param) *cmpVal {
	return &cmpVal{f1, op, val}
}

func Eq(f common.FieldName, val common.Param) *cmpVal { return CmpVal(f, "=", val) }

func Neq(f common.FieldName, val common.Param) Condition { return CmpVal(f, "!=", val) }

func Lt[P comparable](f common.FieldName, val P) Condition { return CmpVal(f, "<", val) }

func Lte[P comparable](f common.FieldName, val P) Condition { return CmpVal(f, "<=", val) }

func Gt[P comparable](f common.FieldName, val P) Condition { return CmpVal(f, ">", val) }

func Gte[P comparable](f common.FieldName, val P) Condition { return CmpVal(f, ">=", val) }

type between[T comparable] struct {
	f          common.Serializable
	start, end T
	isNone     func(T) bool
}

func BetweenInts(f common.FieldName, start, end int) *between[int] {
	return FieldBetweenInts(f, start, end)
}

func FieldBetweenInts(f common.Serializable, start, end int) *between[int] {
	return &between[int]{f: f, start: start, end: end, isNone: func(t int) bool { return t == NoIntLimit }}
}

func BetweenStrings(f common.FieldName, start, end string) *between[string] {
	return FieldBetweenStrings(f, start, end)
}

func FieldBetweenStrings(f common.Serializable, start, end string) *between[string] {
	return &between[string]{f: f, start: start, end: end, isNone: func(t string) bool { return t == NoStringLimit }}
}

func BetweenTimestamps(f common.FieldName, start, end *time.Time) *between[*time.Time] {
	return FieldBetweenTimestamps(f, start, end)
}

func FieldBetweenTimestamps(f common.Serializable, start, end *time.Time) *between[*time.Time] {
	return &between[*time.Time]{f: f, start: start, end: end, isNone: func(t *time.Time) bool { return t == nil || t.IsZero() }}
}

func (c *between[T]) WriteString(in common.CondInterpreter, sb common.Builder) {
	var conds []Condition
	if !c.isNone(c.start) {
		conds = append(conds, CmpVal(c.f, ">=", c.start))
	}
	if !c.isNone(c.end) {
		conds = append(conds, CmpVal(c.f, "<", c.end))
	}
	sb.WriteConditionSerializable(And(conds...), in)
}
