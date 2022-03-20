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
	Commit(b bool) (string, *types.TxReceiptResponseEnvelope, error)
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
}

// SessionManager is a session manager that allows the developer to access orion directly
type SessionManager interface {
	// NewSession creates a new session to orion using the passed identity
	NewSession(id string) (Session, error)
}
