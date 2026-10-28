/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/common"
	drivercommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	sqlcommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
)

type MetadataStore struct {
	*common.MetadataStore
}

func NewMetadataStore(dbs *drivercommon.RWDB, tables common.TableNames) (*MetadataStore, error) {
	return buildMetadataStore(dbs.ReadDB, sqlite.NewRetryWriteDB(dbs.WriteDB), tables.Metadata), nil
}

func buildMetadataStore(readDB *sql.DB, writeDB sqlcommon.WriteDB, table string) *MetadataStore {
	return &MetadataStore{MetadataStore: common.NewMetadataStore(writeDB, readDB, table, &sqlite.ErrorMapper{})}
}
