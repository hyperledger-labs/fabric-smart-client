package common_test

import (
	"context"
	"database/sql"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/common/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/sqlite"
	. "github.com/onsi/gomega"
)

func TestGetData(t *testing.T) {
	RegisterTestingT(t)

	db, mockDB, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	key := "key1"
	data := []byte("value1")
	query := regexp.QuoteMeta("SELECT data FROM test_table WHERE key = $1")

	mockDB.
		ExpectQuery(query).
		WithArgs(key).
		WillReturnRows(mockDB.NewRows([]string{"data"}).AddRow(data))

	store := mockSimpleKeyDataStore(db)
	result, err := store.GetData(context.Background(), key)

	Expect(mockDB.ExpectationsWereMet()).To(Succeed())
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(Equal(data))
}

func TestGetData_NoData(t *testing.T) {
	RegisterTestingT(t)

	db, mockDB, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	key := "missing"
	query := regexp.QuoteMeta("SELECT data FROM test_table WHERE key = $1")

	mockDB.
		ExpectQuery(query).
		WithArgs(key).
		WillReturnRows(mockDB.NewRows([]string{"data"})) // no row

	store := mockSimpleKeyDataStore(db)
	result, err := store.GetData(context.Background(), key)

	Expect(mockDB.ExpectationsWereMet()).To(Succeed())
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(BeNil())
}

func TestExistData_True(t *testing.T) {
	RegisterTestingT(t)

	db, mockDB, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	key := "key1"
	data := []byte("value1")
	query := regexp.QuoteMeta("SELECT data FROM test_table WHERE key = $1")

	mockDB.
		ExpectQuery(query).
		WithArgs(key).
		WillReturnRows(mockDB.NewRows([]string{"data"}).AddRow(data))

	store := mockSimpleKeyDataStore(db)
	exists, err := store.ExistData(context.Background(), key)

	Expect(mockDB.ExpectationsWereMet()).To(Succeed())
	Expect(err).ToNot(HaveOccurred())
	Expect(exists).To(BeTrue())
}

func TestExistData_False(t *testing.T) {
	RegisterTestingT(t)

	db, mockDB, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	key := "missing"
	query := regexp.QuoteMeta("SELECT data FROM test_table WHERE key = $1")

	mockDB.
		ExpectQuery(query).
		WithArgs(key).
		WillReturnRows(mockDB.NewRows([]string{"data"})) // empty result

	store := mockSimpleKeyDataStore(db)
	exists, err := store.ExistData(context.Background(), key)

	Expect(mockDB.ExpectationsWereMet()).To(Succeed())
	Expect(err).ToNot(HaveOccurred())
	Expect(exists).To(BeFalse())
}

func TestPutData_Success(t *testing.T) {
	RegisterTestingT(t)

	db, mockDB, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	key := "key1"
	data := []byte("value1")
	query := regexp.QuoteMeta("INSERT INTO test_table (key, data) VALUES ($1, $2) ON CONFLICT DO NOTHING")

	mockDB.
		ExpectExec(query).
		WithArgs(key, data).
		WillReturnResult(sqlmock.NewResult(1, 1)) // 1 row inserted

	store := mockSimpleKeyDataStore(db)
	err = store.PutData(context.Background(), key, data)

	Expect(mockDB.ExpectationsWereMet()).To(Succeed())
	Expect(err).ToNot(HaveOccurred())
}

func TestPutData_Conflict(t *testing.T) {
	RegisterTestingT(t)

	db, mockDB, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	key := "key1"
	data := []byte("value1")
	query := regexp.QuoteMeta("INSERT INTO test_table (key, data) VALUES ($1, $2) ON CONFLICT DO NOTHING")

	mockDB.
		ExpectExec(query).
		WithArgs(key, data).
		WillReturnResult(sqlmock.NewResult(1, 0)) // conflict, 0 rows affected

	store := mockSimpleKeyDataStore(db)
	err = store.PutData(context.Background(), key, data)

	Expect(mockDB.ExpectationsWereMet()).To(Succeed())
	Expect(err).ToNot(HaveOccurred())
}

func mockSimpleKeyDataStore(db *sql.DB) *common2.SimpleKeyDataStore {
	return common2.NewSimpleKeyDataStore(db, db, "test_table", &mock.SQLErrorWrapper{}, sqlite.NewConditionInterpreter())
}
