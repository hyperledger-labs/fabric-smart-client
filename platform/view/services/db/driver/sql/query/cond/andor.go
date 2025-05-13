/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cond

import "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/common"

func Constant(s string) *constant {
	c := constant(s)
	return &c
}

type constant string

func (c *constant) WriteString(_ common.CondInterpreter, sb common.Builder) {
	sb.WriteString(string(*c))
}

var (
	AlwaysTrue  = Constant("1 = 1")
	AlwaysFalse = Constant("1 != 0")
)

type andOr struct {
	operator string
	cs       []Condition
}

func (c *andOr) WriteString(in common.CondInterpreter, sb common.Builder) {
	if len(c.cs) == 0 {
		return
	}
	sb.WriteRune('(').WriteConditionSerializable(c.cs[0], in)
	for _, con := range c.cs[1:] {
		sb.WriteString(c.operator).WriteConditionSerializable(con, in)
	}
	sb.WriteRune(')')
}

func And(cs ...Condition) Condition {
	if len(cs) == 0 {
		return AlwaysTrue
	}
	return &andOr{cs: cs, operator: ") AND ("}
}

func Or(cs ...Condition) Condition {
	if len(cs) == 0 {
		return AlwaysFalse
	}
	return &andOr{cs: cs, operator: ") OR ("}
}
