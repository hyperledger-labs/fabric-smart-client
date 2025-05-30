/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common_test

import (
	"context"
	"database/sql"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/common/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/sqlite"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	. "github.com/onsi/gomega"
)

type dummyUniqueErrorWrapper struct{}

func (d *dummyUniqueErrorWrapper) WrapError(err error) error {
	return driver.UniqueKeyViolation
}

func TestFilterExistingSigners(t *testing.T) {
	RegisterTestingT(t)

	db, mockDB, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	id1 := view.Identity("signer1")
	id2 := view.Identity("signer2")

	query := regexp.QuoteMeta("SELECT id FROM signer_info WHERE (id) IN (($1), ($2))")

	mockDB.
		ExpectQuery(query).
		WithArgs(id1.UniqueID(), id2.UniqueID()).
		WillReturnRows(mockDB.NewRows([]string{"id"}).
			AddRow(id1.UniqueID()).
			AddRow(id2.UniqueID()))

	store := mockSignerInfoStore(db, false)
	result, err := store.FilterExistingSigners(context.Background(), id1, id2)

	Expect(mockDB.ExpectationsWereMet()).To(Succeed())
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(ContainElements(id1, id2))
}

func TestFilterExistingSigners_QueryError(t *testing.T) {
	RegisterTestingT(t)

	db, mockDB, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	id := view.Identity("signer1")
	query := regexp.QuoteMeta("SELECT id FROM signer_info WHERE id = $1")

	mockDB.
		ExpectQuery(query).
		WithArgs(id.UniqueID()).
		WillReturnError(sql.ErrConnDone)

	store := mockSignerInfoStore(db, false)
	_, err = store.FilterExistingSigners(context.Background(), id)

	Expect(mockDB.ExpectationsWereMet()).To(Succeed())
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("error querying db"))
}

func TestFilterExistingSigners_ScanError(t *testing.T) {
	RegisterTestingT(t)

	db, mockDB, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	id := view.Identity("signer1")
	query := regexp.QuoteMeta("SELECT id FROM signer_info WHERE id = $1")

	mockDB.
		ExpectQuery(query).
		WithArgs(id.UniqueID()).
		WillReturnRows(mockDB.NewRows([]string{"id"}).AddRow(nil)) // nil triggers scan error

	store := mockSignerInfoStore(db, false)
	_, err = store.FilterExistingSigners(context.Background(), id)

	Expect(mockDB.ExpectationsWereMet()).To(Succeed())
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("failed scanning row"))
}

func TestPutSigner(t *testing.T) {
	RegisterTestingT(t)

	db, mockDB, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	id := view.Identity("signer1")

	query := regexp.QuoteMeta("INSERT INTO signer_info (id) VALUES ($1)")

	mockDB.
		ExpectExec(query).
		WithArgs(id.UniqueID()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	store := mockSignerInfoStore(db, false)
	err = store.PutSigner(context.Background(), id)

	Expect(mockDB.ExpectationsWereMet()).To(Succeed())
	Expect(err).ToNot(HaveOccurred())
}

func TestPutSigner_ExecError(t *testing.T) {
	RegisterTestingT(t)

	db, mockDB, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	id := view.Identity("signer1")
	query := regexp.QuoteMeta("INSERT INTO signer_info (id) VALUES ($1)")

	mockDB.
		ExpectExec(query).
		WithArgs(id.UniqueID()).
		WillReturnError(sql.ErrConnDone)

	store := mockSignerInfoStore(db, false)
	err = store.PutSigner(context.Background(), id)

	Expect(mockDB.ExpectationsWereMet()).To(Succeed())
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("failed executing query"))
}

func TestPutSigner_UniqueViolation(t *testing.T) {
	RegisterTestingT(t)

	db, mockDB, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	id := view.Identity("signer1")
	query := regexp.QuoteMeta("INSERT INTO signer_info (id) VALUES ($1)")

	mockDB.
		ExpectExec(query).
		WithArgs(id.UniqueID()).
		WillReturnError(sql.ErrNoRows)

	store := mockSignerInfoStore(db, true)
	err = store.PutSigner(context.Background(), id)

	Expect(mockDB.ExpectationsWereMet()).To(Succeed())
	Expect(err).To(BeNil())
}

func mockSignerInfoStore(db *sql.DB, isdummyUniqueErrorWrapper bool) *common2.SignerInfoStore {
	if isdummyUniqueErrorWrapper {
		return common2.NewSignerInfoStore(db, db, "signer_info", &dummyUniqueErrorWrapper{}, sqlite.NewConditionInterpreter())
	}
	return common2.NewSignerInfoStore(db, db, "signer_info", &mock.SQLErrorWrapper{}, sqlite.NewConditionInterpreter())
}
