/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/common"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	postgres2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
)

type MetadataStore struct {
	*common.MetadataStore
}

func NewMetadataStore(dbs *common2.RWDB, tables common.TableNames) (*MetadataStore, error) {
	return newMetadataStore(dbs.ReadDB, dbs.WriteDB, tables.Metadata), nil
}

func newMetadataStore(readDB, writeDB *sql.DB, table string) *MetadataStore {
	return &MetadataStore{MetadataStore: common.NewMetadataStore(readDB, writeDB, table, &postgres2.ErrorMapper{}, postgres2.NewConditionInterpreter())}
}
