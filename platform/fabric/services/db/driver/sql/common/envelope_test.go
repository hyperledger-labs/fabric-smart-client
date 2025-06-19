/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package common_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/common/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/sqlite"
)

func TestEndorseTX_GetData(t *testing.T) {
	GetData(t, func(db *sql.DB, key string) ([]byte, error) {
		return mockEndorseTXStore(db).GetEndorseTx(context.Background(), key)
	})
}

func TestEndorseTX_GetData_NoData(t *testing.T) {
	GetData_NoData(t, func(db *sql.DB, key string) ([]byte, error) {
		return mockEndorseTXStore(db).GetEndorseTx(context.Background(), key)
	})
}

func TestEndorseTX_ExistData_True(t *testing.T) {
	ExistData_True(t, func(db *sql.DB, key string) (bool, error) {
		return mockEndorseTXStore(db).ExistsEndorseTx(context.Background(), key)
	})
}

func TestEndorseTX_ExistData_False(t *testing.T) {
	ExistData_False(t, func(db *sql.DB, key string) (bool, error) {
		return mockEndorseTXStore(db).ExistsEndorseTx(context.Background(), key)
	})
}

func TestEndorseTX_PutData_Success(t *testing.T) {
	PutData_Success(t, func(db *sql.DB, key string, data []byte) error {
		return mockEndorseTXStore(db).PutEndorseTx(context.Background(), key, data)
	})
}

func TestEndorseTX_PutData_Conflict(t *testing.T) {
	PutData_Conflict(t, func(db *sql.DB, key string, data []byte) error {
		return mockEndorseTXStore(db).PutEndorseTx(context.Background(), key, data)
	})
}

func mockEndorseTXStore(db *sql.DB) *common.EndorseTxStore {
	return common.NewEndorseTxStore(db, db, "test_table", &mock.SQLErrorWrapper{}, sqlite.NewConditionInterpreter())
}
