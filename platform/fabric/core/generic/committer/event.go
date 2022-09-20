/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/peer"
)

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
	ChaincodeID   string
	EventName     string
	Payload       []byte
}

func (chaincodeEvent *ChaincodeEvent) Message() interface{} {
	return chaincodeEvent
}

func (chaincodeEvent *ChaincodeEvent) Topic() string {
	return chaincodeEvent.ChaincodeID
}

func validChaincodeEvent(event *peer.ChaincodeEvent) bool {
	return len(event.GetChaincodeId()) > 0 && len(event.GetEventName()) > 0 && len(event.GetTxId()) > 0
}

func getChaincodeEvent(tx *peer.Transaction, blockNumber uint64) ([]*ChaincodeEvent, error) {
	chaincodeEvents := make([]*ChaincodeEvent, 0)
	for _, action := range tx.GetActions() {
		actionPayload := &peer.ChaincodeActionPayload{}
		if err := proto.Unmarshal(action.GetPayload(), actionPayload); err != nil {
			continue
		}

		responsePayload := &peer.ProposalResponsePayload{}
		if err := proto.Unmarshal(actionPayload.GetAction().GetProposalResponsePayload(), responsePayload); err != nil {
			continue
		}

		action := &peer.ChaincodeAction{}
		if err := proto.Unmarshal(responsePayload.GetExtension(), action); err != nil {
			continue
		}

		event := &peer.ChaincodeEvent{}
		if err := proto.Unmarshal(action.GetEvents(), event); err != nil {
			continue
		}
		if event != nil {
			if !validChaincodeEvent(event) {
				continue
			}

			chaincodeEvent := &ChaincodeEvent{
				BlockNumber:   blockNumber,
				TransactionID: event.GetTxId(),
				ChaincodeID:   event.GetChaincodeId(),
				EventName:     event.GetEventName(),
				Payload:       event.GetPayload(),
			}
			chaincodeEvents = append(chaincodeEvents, chaincodeEvent)
		}

	}

	return chaincodeEvents, nil

}
