/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
)

// FieldName is the name of the DB column
type FieldName string

// Tuple is a tuple of parameters
type Tuple = []Param

// Param is a value for a field
type Param = any

// CondInterpreter is the condition interpreter for the WHERE clauses
// It specifies the behaviors that differ among different DBs
type CondInterpreter interface {
	// TimeOffset appends NOW() - '10 seconds'
	TimeOffset(duration time.Duration, sb Builder)
	// InTuple creates the condition (field1, field2, ...) IN ((val1, val2, ...), (val3, val4, ...))
	InTuple(fields []Serializable, vals []Tuple, sb Builder)
}

// PagInterpreter is the pagination interpreter
type PagInterpreter interface {
	// Interpret modifies the SQL query to add pagination support
	Interpret(p driver.Pagination, sb Builder)
}

// Builder is the string builder
type Builder interface {
	WriteParam(Param) Builder
	WriteTuples([]Tuple) Builder
	WriteString(string) Builder
	WriteRune(rune) Builder
	WriteSerializables(...Serializable) Builder
	WriteConditionSerializable(ConditionSerializable, CondInterpreter) Builder
	Build() (string, []Param)
}

// Serializable is any type can be transformed to a query part, e.g. field, order-by
type Serializable interface {
	WriteString(Builder)
}

// ConditionSerializable is any type that can be transformed to a query part but needs condition interpreter support, e.g. condition, join
type ConditionSerializable interface {
	WriteString(CondInterpreter, Builder)
}
