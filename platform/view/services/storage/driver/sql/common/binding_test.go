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
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/query/common/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	. "github.com/onsi/gomega"
)

func TestGetLongTerm(t *testing.T) {
	RegisterTestingT(t)

	db, mockDB, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	ephemeral := view.Identity("ephemeral_id")
	longTerm := view.Identity("long_term_id")
	expectedQuery := regexp.QuoteMeta("SELECT long_term_id FROM bindings WHERE ephemeral_hash = $1")
	mockDB.
		ExpectQuery(expectedQuery).
		WithArgs(ephemeral.UniqueID()).
		WillReturnRows(mockDB.NewRows([]string{"long_term_id"}).AddRow(longTerm))

	result, err := mockBindingStore(db).GetLongTerm(context.Background(), ephemeral)

	Expect(mockDB.ExpectationsWereMet()).To(Succeed())
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(Equal(longTerm))
}

func TestHaveSameBinding(t *testing.T) {
	RegisterTestingT(t)

	db, mockDB, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	id1 := view.Identity("id1")
	id2 := view.Identity("id2")
	longTerm := view.Identity("long_term_id")

	// Use regexp.QuoteMeta to safely escape the query string
	expectedQuery := regexp.QuoteMeta("SELECT long_term_id FROM bindings WHERE (ephemeral_hash) IN (($1), ($2))")

	mockDB.
		ExpectQuery(expectedQuery).
		WithArgs(id1.UniqueID(), id2.UniqueID()).
		WillReturnRows(mockDB.NewRows([]string{"long_term_id"}).
			AddRow(longTerm).
			AddRow(longTerm),
		)

	result, err := mockBindingStore(db).HaveSameBinding(context.Background(), id1, id2)

	Expect(mockDB.ExpectationsWereMet()).To(Succeed())
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(BeTrue())
}

func TestHaveSameBinding_NotEqual(t *testing.T) {
	RegisterTestingT(t)

	db, mockDB, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	id1 := view.Identity("id1")
	id2 := view.Identity("id2")
	longTerm1 := view.Identity("long_term_id_1")
	longTerm2 := view.Identity("long_term_id_2")

	query := regexp.QuoteMeta("SELECT long_term_id FROM bindings WHERE (ephemeral_hash) IN (($1), ($2))")

	mockDB.
		ExpectQuery(query).
		WithArgs(id1.UniqueID(), id2.UniqueID()).
		WillReturnRows(mockDB.NewRows([]string{"long_term_id"}).
			AddRow(longTerm1).
			AddRow(longTerm2),
		)

	result, err := mockBindingStore(db).HaveSameBinding(context.Background(), id1, id2)

	Expect(mockDB.ExpectationsWereMet()).To(Succeed())
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(BeFalse())
}

func TestHaveSameBinding_MissingEntries(t *testing.T) {
	RegisterTestingT(t)

	db, mockDB, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	id1 := view.Identity("id1")
	id2 := view.Identity("id2")
	longTerm1 := view.Identity("long_term_id_1")

	query := regexp.QuoteMeta("SELECT long_term_id FROM bindings WHERE (ephemeral_hash) IN (($1), ($2))")

	mockDB.
		ExpectQuery(query).
		WithArgs(id1.UniqueID(), id2.UniqueID()).
		WillReturnRows(mockDB.NewRows([]string{"long_term_id"}).
			AddRow(longTerm1), // Only one row returned
		)

	_, err = mockBindingStore(db).HaveSameBinding(context.Background(), id1, id2)

	Expect(mockDB.ExpectationsWereMet()).To(Succeed())
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("1 entries found instead of 2"))
}

func mockBindingStore(db *sql.DB) *common2.BindingStore {
	return common2.NewBindingStore(db, db, "bindings", &mock.SQLErrorWrapper{}, sqlite.NewConditionInterpreter())
}
