/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/common/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/sqlite"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	. "github.com/onsi/gomega"
)

func TestGetAuditInfo(t *testing.T) {
	RegisterTestingT(t)
	db, mockDB, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	input, output := view.Identity("an_id"), []byte("some_result")
	mockDB.
		ExpectQuery("SELECT audit_info FROM audit_info AS audit_info WHERE id = \\$1").
		WithArgs(input.UniqueID()).
		WillReturnRows(mockDB.NewRows([]string{"audit_info"}).AddRow(output))

	info, err := mockAuditInfoStore(db).GetAuditInfo(context.Background(), input)

	Expect(mockDB.ExpectationsWereMet()).To(Succeed())
	Expect(err).ToNot(HaveOccurred())
	Expect(info).To(Equal(output))
}

func TestPutAuditInfo(t *testing.T) {
	RegisterTestingT(t)
	db, mockDB, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	input, output := view.Identity("an_id"), []byte("some_result")
	mockDB.
		ExpectExec("INSERT INTO audit_info \\(id, audit_info\\) VALUES \\(\\$1, \\$2\\)").
		WithArgs(input.UniqueID(), output).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = mockAuditInfoStore(db).PutAuditInfo(context.Background(), input, output)

	Expect(mockDB.ExpectationsWereMet()).To(Succeed())
	Expect(err).ToNot(HaveOccurred())
}

func mockAuditInfoStore(db *sql.DB) *common2.AuditInfoStore {
	return common2.NewAuditInfoStore(db, db, "audit_info", &mock.SQLErrorWrapper{}, sqlite.NewConditionInterpreter())
}
