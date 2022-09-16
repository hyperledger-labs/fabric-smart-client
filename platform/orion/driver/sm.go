/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"github.com/hyperledger-labs/orion-sdk-go/pkg/bcdb"
	"github.com/hyperledger-labs/orion-server/pkg/types"
)

type DataTx interface {
	Put(db string, key string, bytes []byte, a *types.AccessControl) error
	Get(db string, key string) ([]byte, *types.Metadata, error)
	Commit(sync bool) (string, *types.TxReceiptResponseEnvelope, error)
	Delete(db string, key string) error
	SignAndClose() ([]byte, error)
	AddMustSignUser(userID string)
}

type LoadedDataTx interface {
	ID() string
	Commit() error
	CoSignAndClose() ([]byte, error)
	Reads() map[string][]*types.DataRead
	Writes() map[string][]*types.DataWrite
	MustSignUsers() []string
	SignedUsers() []string
}

type Ledger interface {
	NewBlockHeaderDeliveryService(conf *bcdb.BlockHeaderDeliveryConfig) bcdb.BlockHeaderDelivererService
}

// Session let the developer access orion
type Session interface {
	// DataTx returns a data transaction for the passed id
	DataTx(txID string) (DataTx, error)
	LoadDataTx(env *types.DataTxEnvelope) (LoadedDataTx, error)
	Ledger() (Ledger, error)
	Query() (Query, error)
}

// SessionManager is a session manager that allows the developer to access orion directly
type SessionManager interface {
	// NewSession creates a new session to orion using the passed identity
	NewSession(id string) (Session, error)
}

// QueryIterator is an iterator over the results of a query
type QueryIterator = bcdb.Iterator

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
