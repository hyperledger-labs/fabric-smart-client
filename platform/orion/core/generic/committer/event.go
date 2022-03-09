/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

// TxEvent contains information for token transaction commit
type TxEvent struct {
	Txid           string
	DependantTxIDs []string
	Committed      bool
	Block          uint64
	IndexInBlock   int
	CommitPeer     string
	Err            error
}
