/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"database/sql"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
)

type MetadataStore struct {
	*common.MetadataStore
}

func NewMetadataStore(opts Opts) (*MetadataStore, error) {
	dbs, err := DbProvider.OpenDB(opts)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	tables := common.GetTableNames(opts.TablePrefix, opts.TableNameParams...)
	return newMetadataStore(dbs.ReadDB, NewRetryWriteDB(dbs.WriteDB), tables.Metadata), nil
}

func newMetadataStore(readDB *sql.DB, writeDB common.WriteDB, table string) *MetadataStore {
	return &MetadataStore{MetadataStore: common.NewMetadataStore(writeDB, readDB, table, &errorMapper{}, NewInterpreter())}
}
