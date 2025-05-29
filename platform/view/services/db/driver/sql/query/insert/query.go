/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package _insert

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/common"
)

type query struct {
	table          common.TableName
	fields         []common.FieldName
	rows           []common.Tuple
	conflictFields []common.FieldName
	onConflicts    []OnConflict
	ignoreConflict bool
}

func NewQuery() *query {
	return &query{}
}

func (q *query) Into(table common.TableName) Query {
	q.table = table
	return q
}

func (q *query) Fields(fields ...common.FieldName) fieldsQuery {
	q.fields = fields
	return q
}

func (q *query) Row(tuple ...common.Param) fieldsQuery {
	if len(tuple) != len(q.fields) {
		panic("wrong length")
	}
	q.rows = append(q.rows, tuple)
	return q
}

func (q *query) Rows(tuples []common.Tuple) fieldsQuery {
	for _, tuple := range tuples {
		q.Row(tuple...)
	}
	return q
}

func (q *query) OnConflict(fields []common.FieldName, onConflicts ...OnConflict) onConflictQuery {
	if len(onConflicts) == 0 {
		panic("no strategy passed")
	}
	q.conflictFields = fields
	q.onConflicts = onConflicts
	return q
}

func (q *query) OnConflictDoNothing() onConflictQuery {
	q.ignoreConflict = true
	return q
}

func (q *query) Format() (string, []common.Param) {
	sb := common.NewBuilder()
	q.FormatTo(sb)
	return sb.Build()
}

func (q *query) FormatTo(sb common.Builder) {
	sb.WriteString("INSERT INTO ").
		WriteString(string(q.table)).
		WriteString(" (").
		WriteSerializables(common.ToSerializables(q.fields)...).
		WriteString(") VALUES ").
		WriteTuples(q.rows)

	if q.ignoreConflict {
		sb.WriteString(" ON CONFLICT DO NOTHING")
		return
	}
	if q.conflictFields != nil {
		sb.WriteString(" ON CONFLICT (").
			WriteSerializables(common.ToSerializables(q.conflictFields)...).
			WriteString(") DO UPDATE SET ").
			WriteSerializables(common.ToSerializables(q.onConflicts)...)
	}
}
