/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package _insert

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/query/common"
)

// Query is the query state after INSERT INTO
type Query interface {
	// Fields specifies the fields to insert
	Fields(...common.FieldName) fieldsQuery
}

// fieldsQuery is the query state after the fields
type fieldsQuery interface {
	onConflictQuery

	// Row adds a row to insert
	Row(...common.Param) fieldsQuery

	// Rows adds multiple rows to insert
	Rows([]common.Tuple) fieldsQuery

	// OnConflict specifies the ON CONFLICT DO UPDATE clause
	OnConflict([]common.FieldName, ...OnConflict) onConflictQuery

	// OnConflictDoNothing specifies ON CONFLICT DO NOTHING
	OnConflictDoNothing() onConflictQuery
}

// OnConflict specifies what to do when there is a conflict during insertion
type OnConflict common.Serializable

// onConflictQuery is the query state after ON CONFLICT
type onConflictQuery interface {
	// Format composes the query and the params to pass to the DB
	Format() (string, []common.Param)

	// FormatTo composes the query and the params to pass to the DB with an offset for the numbered params
	FormatTo(common.Builder)
}
