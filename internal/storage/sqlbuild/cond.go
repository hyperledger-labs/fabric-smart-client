/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlbuild

// Condition is a fragment of a WHERE clause. Implementations write themselves
// into a [Builder], taking their placeholder numbers from it.
type Condition interface {
	WriteTo(*Builder)
}

// CondFunc adapts a function to [Condition]. Use it for the rare fragment the
// constructors in this package do not cover, such as a scalar subquery.
type CondFunc func(*Builder)

// WriteTo implements [Condition].
func (f CondFunc) WriteTo(b *Builder) { f(b) }

// cmp is a binary comparison between a column and a single value.
type cmp struct {
	col string
	op  string
	val Param
}

// WriteTo implements Condition.
func (c cmp) WriteTo(b *Builder) {
	b.WriteString(c.col).WriteString(c.op).WriteParam(c.val)
}

// Eq renders "col = $n".
func Eq(col string, val Param) Condition { return cmp{col: col, op: " = ", val: val} }

// Gt renders "col > $n".
func Gt(col string, val Param) Condition { return cmp{col: col, op: " > ", val: val} }

// Gte renders "col >= $n".
func Gte(col string, val Param) Condition { return cmp{col: col, op: " >= ", val: val} }

// Lt renders "col < $n".
func Lt(col string, val Param) Condition { return cmp{col: col, op: " < ", val: val} }

// in is a set membership test.
type in struct {
	col  string
	vals []Param
}

// In renders "col IN ($n,…)". A single value still renders as IN ($n) rather
// than "col = $n"; callers that want the equality form call [Eq]. With no values
// it renders the always-false "(1=0)", so an empty key set matches no rows.
func In[V any](col string, vals ...V) Condition {
	params := make([]Param, len(vals))
	for i, v := range vals {
		params[i] = v
	}
	return in{col: col, vals: params}
}

// WriteTo implements Condition.
func (c in) WriteTo(b *Builder) {
	if len(c.vals) == 0 {
		b.WriteString("(1=0)")
		return
	}
	b.WriteString(c.col).WriteString(" IN (")
	for i, v := range c.vals {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteParam(v)
	}
	b.WriteString(")")
}

// writeJoined writes the non-nil conditions to b separated by " AND " and
// reports whether it wrote anything.
func writeJoined(b *Builder, conds []Condition) bool {
	wrote := false
	for _, cond := range conds {
		if cond == nil {
			continue
		}
		if wrote {
			b.WriteString(" AND ")
		}
		cond.WriteTo(b)
		wrote = true
	}
	return wrote
}

// and is a conjunction of conditions.
type and []Condition

// And renders "(a AND b AND …)", keeping the parentheses even for a single
// condition. nil conditions are skipped, so optional bounds can be passed
// through directly. With nothing left it renders the always-true "(1=1)".
func And(conds ...Condition) Condition { return and(conds) }

// WriteTo implements Condition.
func (c and) WriteTo(b *Builder) {
	b.WriteString("(")
	if !writeJoined(b, c) {
		b.WriteString("1=1")
	}
	b.WriteString(")")
}
