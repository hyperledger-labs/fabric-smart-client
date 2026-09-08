/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"database/sql"

	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
)

type SignerInfoStore struct {
	*common2.SignerInfoStore
}

func NewSignerInfoStore(dbs *common3.RWDB, tables common2.TableNames) (*SignerInfoStore, error) {
	return buildSignerInfoStore(dbs.ReadDB, NewRetryWriteDB(dbs.WriteDB), tables.SignerInfo), nil
}

func buildSignerInfoStore(readDB *sql.DB, writeDB common2.WriteDB, table string) *SignerInfoStore {
	return &SignerInfoStore{SignerInfoStore: common2.NewSignerInfoStore(writeDB, readDB, table, &ErrorMapper{})}
}
