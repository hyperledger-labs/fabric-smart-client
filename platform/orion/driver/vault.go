/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

// Vault models a key value store that can be updated by committing rwsets
type Vault interface {
	GetLastTxID() (string, error)
	NewQueryExecutor() (QueryExecutor, error)
	NewRWSet(txid string) (RWSet, error)
	GetRWSet(id string, results []byte) (RWSet, error)
}
