/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/common"
	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	sqlite2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
)

type MetadataStore struct {
	*common.MetadataStore
}

func NewMetadataStore(dbs *common3.RWDB, tables common.TableNames) (*MetadataStore, error) {
	return newMetadataStore(dbs.ReadDB, sqlite2.NewRetryWriteDB(dbs.WriteDB), tables.Metadata), nil
}

func newMetadataStore(readDB *sql.DB, writeDB common2.WriteDB, table string) *MetadataStore {
	return &MetadataStore{MetadataStore: common.NewMetadataStore(writeDB, readDB, table, &sqlite2.ErrorMapper{}, sqlite2.NewConditionInterpreter())}
}
