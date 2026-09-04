/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/mock"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func TestGetLongTerm(t *testing.T) { //nolint:paralleltest
	RegisterTestingT(t)

	db, mockDB, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	Expect(err).ToNot(HaveOccurred())

	ephemeral := view.Identity("ephemeral_id")
	longTerm := view.Identity("long_term_id")
	expectedQuery := "SELECT long_term_id FROM bindings WHERE ephemeral_hash = $1"
	mockDB.
		ExpectQuery(expectedQuery).
		WithArgs(ephemeral.UniqueID()).
		WillReturnRows(mockDB.NewRows([]string{"long_term_id"}).AddRow(longTerm))

	result, err := mockBindingStore(db).GetLongTerm(context.Background(), ephemeral)

	Expect(mockDB.ExpectationsWereMet()).To(Succeed())
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(Equal(longTerm))
}

func TestHaveSameBinding(t *testing.T) { //nolint:paralleltest
	RegisterTestingT(t)

	db, mockDB, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	Expect(err).ToNot(HaveOccurred())

	id1 := view.Identity("id1")
	id2 := view.Identity("id2")
	longTerm := view.Identity("long_term_id")

	expectedQuery := "SELECT long_term_id FROM bindings WHERE ephemeral_hash IN ($1,$2)"

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

func TestHaveSameBinding_NotEqual(t *testing.T) { //nolint:paralleltest
	RegisterTestingT(t)

	db, mockDB, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	Expect(err).ToNot(HaveOccurred())

	id1 := view.Identity("id1")
	id2 := view.Identity("id2")
	longTerm1 := view.Identity("long_term_id_1")
	longTerm2 := view.Identity("long_term_id_2")

	query := "SELECT long_term_id FROM bindings WHERE ephemeral_hash IN ($1,$2)"

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

func TestHaveSameBinding_MissingEntries(t *testing.T) { //nolint:paralleltest
	RegisterTestingT(t)

	db, mockDB, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	Expect(err).ToNot(HaveOccurred())

	id1 := view.Identity("id1")
	id2 := view.Identity("id2")
	longTerm1 := view.Identity("long_term_id_1")

	query := "SELECT long_term_id FROM bindings WHERE ephemeral_hash IN ($1,$2)"

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

// PutBindings resolves the canonical long-term id first, then writes one
// multi-row INSERT whose first tuple is the long-term id bound to itself.
func TestPutBindings(t *testing.T) { //nolint:paralleltest
	RegisterTestingT(t)

	expectPutBindings(t,
		"INSERT INTO bindings (ephemeral_hash, long_term_id) "+
			"VALUES ($1,$2),($3,$4) ON CONFLICT DO NOTHING;",
		view.Identity("eph1"))
}

func TestPutBindings_SeveralEphemerals(t *testing.T) { //nolint:paralleltest
	RegisterTestingT(t)

	expectPutBindings(t,
		"INSERT INTO bindings (ephemeral_hash, long_term_id) "+
			"VALUES ($1,$2),($3,$4),($5,$6) ON CONFLICT DO NOTHING;",
		view.Identity("eph1"), view.Identity("eph2"))
}

// expectPutBindings pins the statement PutBindings emits for ephemerals, along
// with the arguments each placeholder stands for.
func expectPutBindings(t *testing.T, query string, ephemerals ...view.Identity) {
	t.Helper()

	db, mockDB, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	Expect(err).ToNot(HaveOccurred())

	longTerm := view.Identity("long_term")

	// the canonical-id lookup returns no row, so longTerm is used as given
	mockDB.
		ExpectQuery("SELECT long_term_id FROM bindings WHERE ephemeral_hash = $1").
		WithArgs(longTerm.UniqueID()).
		WillReturnRows(mockDB.NewRows([]string{"long_term_id"}))

	args := []driver.Value{longTerm.UniqueID(), []byte(longTerm)}
	for _, eph := range ephemerals {
		args = append(args, eph.UniqueID(), []byte(longTerm))
	}
	mockDB.
		ExpectExec(query).
		WithArgs(args...).
		WillReturnResult(sqlmock.NewResult(1, int64(len(ephemerals)+1)))

	Expect(mockBindingStore(db).PutBindings(context.Background(), longTerm, ephemerals...)).To(Succeed())
	Expect(mockDB.ExpectationsWereMet()).To(Succeed())
}

func mockBindingStore(db *sql.DB) *common2.BindingStore {
	return common2.NewBindingStore(db, db, "bindings", &mock.SQLErrorWrapper{})
}
