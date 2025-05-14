/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package query

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/common"
	_delete "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/delete"
	_insert "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/insert"
	_select "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/select"
	_update "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/update"
)

// Select initiates a SELECT query
func Select() _select.Query { return _select.NewQuery(false) }

// SelectDistinct initiates a SELECT DISTINCT query
func SelectDistinct() _select.Query { return _select.NewQuery(true) }

// Table creates a Table instance without assigning any alias
func Table(name string) common.Table { return common.NewAliasedTable(common.TableName(name)) }

// Asc creates an ORDER BY field ASC clause
func Asc(name common.Field) _select.OrderBy { return _select.Asc(name) }

// Desc creates an ORDER BY field DESC clause
func Desc(name common.Field) _select.OrderBy { return _select.Desc(name) }

// Update initiates an UPDATE query
func Update(t string) _update.Query {
	return _update.NewQuery().Update(common.TableName(t))
}

// InsertInto initiates an INSERT INTO query
func InsertInto(t string) _insert.Query { return _insert.NewQuery().Into(common.TableName(t)) }

// SetValue creates a SET within an ON CONFLICT clause to set a field to a new fixed value
func SetValue(field common.FieldName, value common.Param) _insert.OnConflict {
	return _insert.Set(field, value)
}

// OverwriteValue creates a SET within an ON CONFLICT clause to overwrite the field
func OverwriteValue(field common.FieldName) _insert.OnConflict { return _insert.Overwrite(field) }

// DeleteFrom initiates a DELETE query
func DeleteFrom(t string) _delete.Query { return _delete.NewQuery().From(common.TableName(t)) }
