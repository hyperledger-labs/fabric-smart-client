/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlbuild

import "strconv"

// Paging is the contribution a pagination makes to a SELECT statement.
//
// The zero value contributes nothing, which is why Limit is a pointer rather
// than an int with a sentinel: forgetting to set it yields an unpaginated read
// rather than a silently empty result set. LIMIT 0 stays expressible as new(0),
// because it is a meaningful query that returns no rows.
//
// Where is not written by [Builder.WritePaging]: it has to be merged into the
// statement's WHERE clause, which comes before ORDER BY, so the caller passes
// it to [Builder.WriteWhere] alongside its own conditions.
type Paging struct {
	// OrderBy is the column to sort ascending by, or "" for no ORDER BY.
	OrderBy string
	// Limit caps the rows returned. A nil Limit — or one pointing at a negative
	// value — writes no LIMIT clause.
	Limit *int
	// Offset is the number of rows to skip. It is written only when Limit is
	// set and Offset is positive.
	Offset int
	// Where is the keyset cursor condition, or nil.
	Where Condition
}

// WritePaging appends the ORDER BY, LIMIT and OFFSET clauses p calls for, in
// that order. LIMIT and OFFSET are written as literals rather than parameters,
// so they do not appear in the parameters [Builder.Build] returns.
func (b *Builder) WritePaging(p Paging) *Builder {
	if p.OrderBy != "" {
		b.WriteString(" ORDER BY ").WriteString(p.OrderBy).WriteString(" ASC")
	}
	if p.Limit == nil || *p.Limit < 0 {
		return b
	}
	b.WriteString(" LIMIT ").WriteString(strconv.Itoa(*p.Limit))
	if p.Offset > 0 {
		b.WriteString(" OFFSET ").WriteString(strconv.Itoa(p.Offset))
	}
	return b
}
