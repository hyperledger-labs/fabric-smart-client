/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package sqlbuild assembles the handful of SQL statement shapes FSC's storage
// drivers need, together with their positional parameters.
//
// It is deliberately not a query builder: there is no support for joins,
// aliases, DISTINCT, GROUP BY, HAVING or subqueries. Callers write the static
// parts of their statement as plain strings and use this package only for the
// parts that vary — parameter numbering, value tuples, WHERE conditions and
// pagination suffixes.
//
// Placeholders are always $N, numbered in write order. Both PostgreSQL and
// modernc.org/sqlite bind $N by number, so the same statement text works on
// both.
package sqlbuild

import (
	"slices"
	"strconv"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// Param is a single positional statement parameter.
type Param = any

// Tuple is one row of values for a multi-row INSERT.
type Tuple = []Param

// ErrNoValues reports an INSERT with no rows to insert. Callers return it
// instead of building the statement; [Builder.WriteTuples] panics if they do
// not. The message is squirrel's, so the error a caller surfaces for an empty
// insert is unchanged from before this package existed.
var ErrNoValues = errors.New("insert statements must have at least one set of values or select clause")

// Builder accumulates a SQL statement and its positional parameters.
// The zero value is not usable; call [New].
type Builder struct {
	sb     strings.Builder
	params []Param
}

// New returns an empty [Builder].
func New() *Builder {
	return &Builder{}
}

// WriteString appends raw SQL text verbatim. The text is never escaped or
// inspected, so it must not contain caller-supplied values — use
// [Builder.WriteParam] for those.
func (b *Builder) WriteString(s string) *Builder {
	b.sb.WriteString(s)
	return b
}

// WriteParam appends the next placeholder ($1, $2, …) and records v as its
// value.
func (b *Builder) WriteParam(v Param) *Builder {
	b.sb.WriteByte('$')
	b.sb.WriteString(strconv.Itoa(len(b.params) + 1))
	b.params = append(b.params, v)
	return b
}

// WriteTuples appends the VALUES rows of an INSERT as ($1,$2),($3,$4), with no
// spaces around the separators. It panics when rows is empty: there is no SQL
// for a VALUES clause with no rows, so callers reject that with [ErrNoValues]
// before they start building.
func (b *Builder) WriteTuples(rows []Tuple) *Builder {
	if len(rows) == 0 {
		panic("sqlbuild: WriteTuples called with no rows; callers must reject this with ErrNoValues")
	}
	for i, row := range rows {
		if i > 0 {
			b.sb.WriteByte(',')
		}
		b.sb.WriteByte('(')
		for j, v := range row {
			if j > 0 {
				b.sb.WriteByte(',')
			}
			b.WriteParam(v)
		}
		b.sb.WriteByte(')')
	}
	return b
}

// Build returns the accumulated statement and its parameters. The parameter
// slice is nil when the statement takes no parameters.
func (b *Builder) Build() (string, []Param) {
	return b.sb.String(), b.params
}

// WriteWhere appends " WHERE " followed by the non-nil conditions joined with
// " AND ", or nothing at all when none are given. The conditions are joined
// without wrapping parentheses; use [And] where a bracketed group is needed.
func (b *Builder) WriteWhere(conds ...Condition) *Builder {
	if !slices.ContainsFunc(conds, func(c Condition) bool { return c != nil }) {
		return b
	}
	b.WriteString(" WHERE ")
	writeJoined(b, conds)
	return b
}
