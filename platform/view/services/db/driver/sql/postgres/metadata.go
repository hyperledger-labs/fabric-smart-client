/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

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
	return newMetadataStore(dbs.ReadDB, dbs.WriteDB, tables.Metadata), nil
}

func newMetadataStore(readDB, writeDB *sql.DB, table string) *MetadataStore {
	return &MetadataStore{MetadataStore: common.NewMetadataStore(readDB, writeDB, table, &errorMapper{}, NewInterpreter())}
}
