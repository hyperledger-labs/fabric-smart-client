/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sql

import (
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
)

const (
	Postgres       common.SQLDriverType    = "postgres"
	SQLite         common.SQLDriverType    = "sqlite"
	SQLPersistence driver2.PersistenceType = "sql"
)
