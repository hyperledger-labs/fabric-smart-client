/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"github.com/hyperledger-labs/orion-server/pkg/types"
)

type DataTx interface {
	Put(db string, key string, bytes []byte, a *types.AccessControl) error
	Get(db string, key string) ([]byte, *types.Metadata, error)
	Commit(b bool) (string, *types.TxReceipt, error)
}

type Session interface {
	DataTx(txID string) (DataTx, error)
}

type SessionManager interface {
	NewSession(id string) (Session, error)
}
