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

type ChaincodeEvent struct {
	BlockNumber   uint64
	TransactionID string
	ChaincodeName string
	EventName     string
	Payload       []byte
}

func (chaincodeEvent ChaincodeEvent) Message() interface{} {
	return &chaincodeEvent
}

func (chaincodeEvent ChaincodeEvent) Topic() string {
	return chaincodeEvent.ChaincodeName
}
