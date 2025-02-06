/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"golang.org/x/exp/constraints"
)

const (
	NoStringLimit = ""
	NoIntLimit    = -1
)

type IsolationLevelMapper interface {
	Map(level driver.IsolationLevel) (sql.IsolationLevel, error)
}

type Interpreter interface {
	Cmp(field FieldName, symbol string, value any) Condition
	And(conditions ...Condition) Condition
	Or(conditions ...Condition) Condition
	In(field FieldName, vals []any) Condition
	InStrings(field FieldName, vals []string) Condition
	InInts(field FieldName, vals []int) Condition
	InTuple(fields []FieldName, vals []Tuple) Condition
	BetweenStrings(field FieldName, startKey, endKey string) Condition
	BetweenInts(field FieldName, startKey, endKey int) Condition
}

type FieldName = string

type Tuple = []any

type Condition interface {
	ToString(*int) string
	isEmpty() bool
	Params() []any
}

var EmptyCondition = emptyCondition{}

func NewInterpreter() *interpreter {
	return &interpreter{}
}

func Where(c Condition) (string, []any) {
	ctr := 1
	w := c.ToString(&ctr)
	if len(w) == 0 {
		return "", []any{}
	}
	return "WHERE " + w, c.Params()
}

func JoinCol(tableName string, field FieldName) string {
	if len(tableName) > 0 {
		return fmt.Sprintf("%s.%s", tableName, field)
	}
	return field
}

type interpreter struct{}

func (i *interpreter) Cmp(field FieldName, symbol string, value any) Condition {
	if s, ok := value.(string); ok && len(s) == 0 {
		return EmptyCondition
	}
	return &compareCondition{symbol: symbol, field: field, val: value}
}

func (i *interpreter) And(conditions ...Condition) Condition {
	return newAndOrCondition("AND", conditions...)
}

func (i *interpreter) Or(conditions ...Condition) Condition {
	return newAndOrCondition("OR", conditions...)
}

func (i *interpreter) In(field FieldName, vals []any) Condition {
	tuples := make([]Tuple, len(vals))
	for i, val := range vals {
		tuples[i] = Tuple{val}
	}
	return i.InTuple([]FieldName{field}, tuples)
}

func (i *interpreter) InStrings(field FieldName, vals []string) Condition {
	tuples := make([]Tuple, len(vals))
	for i, val := range vals {
		tuples[i] = Tuple{val}
	}
	return i.InTuple([]FieldName{field}, tuples)
}

func (i *interpreter) InInts(field FieldName, vals []int) Condition {
	tuples := make([]Tuple, len(vals))
	for i, val := range vals {
		tuples[i] = Tuple{val}
	}
	return i.InTuple([]FieldName{field}, tuples)
}

func (i *interpreter) InTuple(fields []FieldName, vals []Tuple) Condition {
	if len(vals) == 0 || len(fields) == 0 {
		return EmptyCondition
	} else if len(vals) == 1 && len(fields) == 1 {
		// Not necessary, but makes the query more readable
		return i.Cmp(fields[0], "=", vals[0][0])
	}
	return &inCondition{fields: fields, vals: vals}
}

func (i *interpreter) BetweenStrings(field FieldName, startKey, endKey string) Condition {
	return between(i, field, startKey, endKey, NoStringLimit)
}

func (i *interpreter) BetweenInts(field FieldName, startKey, endKey int) Condition {
	return between(i, field, startKey, endKey, NoIntLimit)
}

func between[T comparable](i *interpreter, field FieldName, startKey, endKey, nilVal T) Condition {
	var conds []Condition
	if startKey != nilVal {
		conds = append(conds, i.Cmp(field, ">=", startKey))
	}
	if endKey != nilVal {
		conds = append(conds, i.Cmp(field, "<", endKey))
	}
	return i.And(conds...)
}

func newAndOrCondition(keyword string, conds ...Condition) Condition {
	nonEmpty := make([]Condition, 0)
	for _, c := range conds {
		if !c.isEmpty() {
			nonEmpty = append(nonEmpty, c)
		}
	}
	if len(nonEmpty) == 0 {
		return EmptyCondition
	}
	return &andOrCondition{keyword: " " + keyword + " ", conds: nonEmpty}
}

type andOrCondition struct {
	conds   []Condition
	keyword string
}

func (c *andOrCondition) ToString(ctr *int) string {
	sb := strings.Builder{}
	sb.WriteString(c.conds[0].ToString(ctr))
	for i := 1; i < len(c.conds); i++ {
		sb.WriteString(c.keyword)
		sb.WriteString(c.conds[i].ToString(ctr))
	}
	return "(" + sb.String() + ")"
}

func (c *andOrCondition) isEmpty() bool {
	return len(c.conds) == 0
}

func (c *andOrCondition) Params() []any {
	params := make([]any, 0)
	for _, cond := range c.conds {
		params = append(params, cond.Params()...)
	}
	return params
}

type emptyCondition struct{}

func (c emptyCondition) ToString(*int) string { return "" }

func (c emptyCondition) Params() []any { return []any{} }

func (c emptyCondition) isEmpty() bool { return true }

type ConstCondition string

func (c ConstCondition) ToString(*int) string { return string(c) }

func (c ConstCondition) Params() []any { return []any{} }

func (c ConstCondition) isEmpty() bool { return false }

type compareCondition struct {
	symbol string
	field  FieldName
	val    any
}

func (c *compareCondition) ToString(ctr *int) string {
	r := fmt.Sprintf("%s %s $%d", c.field, c.symbol, *ctr)
	*ctr += 1
	return r
}

func (c *compareCondition) Params() []any { return []any{c.val} }

func (c *compareCondition) isEmpty() bool { return false }

type inCondition struct {
	fields []FieldName
	vals   []Tuple
}

func (c *inCondition) ToString(ctr *int) string {
	sb := &strings.Builder{}
	sb.WriteString("(")
	sb.WriteString(strings.Join(c.fields, ", "))
	sb.WriteString(") IN (")
	appendParamsMatrix(sb, len(c.fields), len(c.vals), *ctr)
	sb.WriteString(")")
	*ctr += len(c.fields) * len(c.vals)

	return sb.String()
}

func CreateParamsMatrix(fields, rows int, offset int) string {
	sb := &strings.Builder{}
	appendParamsMatrix(sb, fields, rows, offset)
	return sb.String()
}

func appendParamsMatrix(sb *strings.Builder, fields, rows int, offset int) {
	sb.WriteString("($")
	sb.WriteString(strconv.Itoa(offset))
	offset++
	for j := 1; j < fields; j++ {
		sb.WriteString(", $")
		sb.WriteString(strconv.Itoa(offset))
		offset++
	}
	sb.WriteString(")")
	for i := 1; i < rows; i++ {
		sb.WriteString(", ($")
		sb.WriteString(strconv.Itoa(offset))
		offset++
		for j := 1; j < fields; j++ {
			sb.WriteString(", $")
			sb.WriteString(strconv.Itoa(offset))
			offset++
		}
		sb.WriteString(")")
	}
}

func (c *inCondition) Params() []any {
	params := make([]any, len(c.fields)*len(c.vals))
	for i, t := range c.vals {
		for j, v := range t {
			params[i*len(t)+j] = v
		}
	}
	return params
}

func (c *inCondition) isEmpty() bool { return false }

func ToInts[T constraints.Integer](in []T) []int {
	r := make([]int, len(in))
	for i, v := range in {
		r[i] = int(v)
	}
	return r
}

func ToAnys[T any](in []T) []any {
	r := make([]any, len(in))
	for i, v := range in {
		r[i] = v
	}
	return r
}
