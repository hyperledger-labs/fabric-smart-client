/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"database/sql"

	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
)

type MetadataStore struct {
	*common.MetadataStore
}

func NewMetadataStore(dbs *common3.RWDB, tables common.TableNames) (*MetadataStore, error) {
	return newMetadataStore(dbs.ReadDB, NewRetryWriteDB(dbs.WriteDB), tables.Metadata), nil
}

func newMetadataStore(readDB *sql.DB, writeDB common.WriteDB, table string) *MetadataStore {
	return &MetadataStore{MetadataStore: common.NewMetadataStore(writeDB, readDB, table, &errorMapper{}, NewConditionInterpreter())}
}
