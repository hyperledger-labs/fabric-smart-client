/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/common"
	drivercommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	sqlcommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
)

type MetadataStore struct {
	*common.MetadataStore
}

func NewMetadataStore(dbs *drivercommon.RWDB, tables common.TableNames) (*MetadataStore, error) {
	return buildMetadataStore(dbs.ReadDB, dbs.WriteDB, tables.Metadata, sqlcommon.QueryTimeout(dbs.QueryTimeout)), nil
}

func buildMetadataStore(readDB, writeDB *sql.DB, table string, timeout sqlcommon.QueryTimeout) *MetadataStore {
	return &MetadataStore{MetadataStore: common.NewMetadataStore(writeDB, readDB, table, &postgres.ErrorMapper{}, timeout)}
}
