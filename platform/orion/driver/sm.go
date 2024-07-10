/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

type DataRead struct {
	Key string
}

type DataWrite struct {
	Key   string
	Value []byte
	Acl   interface{}
}

type AccessControl = interface{}

type Flag = int32

const (
	VALID Flag = iota
)

type DataTx interface {
	Put(db string, key string, bytes []byte, a AccessControl) error
	Get(db string, key string) ([]byte, error)
	Commit(sync bool) (string, error)
	Delete(db string, key string) error
	SignAndClose() ([]byte, error)
	AddMustSignUser(userID string)
}

type LoadedDataTx interface {
	ID() string
	Commit() error
	CoSignAndClose() ([]byte, error)
	Reads() map[string][]*DataRead
	Writes() map[string][]*DataWrite
	MustSignUsers() []string
	SignedUsers() []string
}

type Ledger interface {
	GetTransactionReceipt(txId string) (Flag, error)
}

// Session let the developer access orion
// Any implementation must be thread-safe
type Session interface {
	// DataTx returns a data transaction for the passed id
	DataTx(txID string) (DataTx, error)
	LoadDataTx(env interface{}) (LoadedDataTx, error)
	Ledger() (Ledger, error)
	Query() (Query, error)
}

// SessionManager is a session manager that allows the developer to access orion directly
type SessionManager interface {
	// NewSession creates a new session to orion using the passed identity
	NewSession(id string) (Session, error)
}

// QueryIterator is an iterator over the results of a query
type QueryIterator interface {
	// Next returns the next record. If there is no more records, it would return a nil value
	// and a false value.
	Next() (string, []byte, uint64, uint64, bool, error)
}

// Query allows the developer to perform queries over the orion database
type Query interface {
	// GetDataByRange executes a range query on a given database. The startKey is
	// inclusive but endKey is not. When the startKey is an empty string, it denotes
	// `fetch keys from the beginning` while an empty endKey denotes `fetch keys till the
	// the end`. The limit denotes the number of records to be fetched in total. However,
	// when the limit is set to 0, it denotes no limit. The iterator returned by
	// GetDataByRange is used to retrieve the records.
	GetDataByRange(dbName, startKey, endKey string, limit uint64) (QueryIterator, error)
}
