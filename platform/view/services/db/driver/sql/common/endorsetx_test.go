/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package common_test

import (
	"context"
	"database/sql"
	"testing"

	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/common/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/sqlite"
)

func TestEnvelope_GetData(t *testing.T) {
	GetData(t, func(db *sql.DB, key string) ([]byte, error) {
		return mockEnvelopeStore(db).GetEnvelope(context.Background(), key)
	})
}

func TestEnvelope_GetData_NoData(t *testing.T) {
	GetData_NoData(t, func(db *sql.DB, key string) ([]byte, error) {
		return mockEnvelopeStore(db).GetEnvelope(context.Background(), key)
	})
}

func TestEnvelope_ExistData_True(t *testing.T) {
	ExistData_True(t, func(db *sql.DB, key string) (bool, error) {
		return mockEnvelopeStore(db).ExistsEnvelope(context.Background(), key)
	})
}

func TestEnvelope_ExistData_False(t *testing.T) {
	ExistData_False(t, func(db *sql.DB, key string) (bool, error) {
		return mockEnvelopeStore(db).ExistsEnvelope(context.Background(), key)
	})
}

func TestEnvelope_PutData_Success(t *testing.T) {
	PutData_Success(t, func(db *sql.DB, key string, data []byte) error {
		return mockEnvelopeStore(db).PutEnvelope(context.Background(), key, data)
	})
}

func TestEnvelope_PutData_Conflict(t *testing.T) {
	PutData_Conflict(t, func(db *sql.DB, key string, data []byte) error {
		return mockEnvelopeStore(db).PutEnvelope(context.Background(), key, data)
	})
}

func mockEnvelopeStore(db *sql.DB) *common2.EnvelopeStore {
	return common2.NewEnvelopeStore(db, db, "test_table", &mock.SQLErrorWrapper{}, sqlite.NewConditionInterpreter())
}
