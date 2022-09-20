/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/protoutil"
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

func getChaincodeEvent(env *common.Envelope, blockNumber uint64) ([]*ChaincodeEvent, error) {
	chaincodeEvents := make([]*ChaincodeEvent, 0)
	chaincodeActions, err := protoutil.GetActionsFromEnvelopeMsg(env)
	if err != nil {
		logger.Errorf("Error getting chaincode actions from envelop: %s", err)
		return nil, err
	}
	for _, action := range chaincodeActions {

		if action == nil {
			continue
		}
		chaincodeEventData, err := protoutil.UnmarshalChaincodeEvents(action.GetEvents())

		if err != nil {
			logger.Errorf("Error getting chaincode event from chaincode actions: %s", err)
			return nil, err
		}

		if chaincodeEventData != nil {
			if !validChaincodeEvent(chaincodeEventData) {
				continue
			}
			chaincodeEvent := &ChaincodeEvent{
				BlockNumber:   blockNumber,
				TransactionID: chaincodeEventData.GetTxId(),
				ChaincodeID:   chaincodeEventData.GetChaincodeId(),
				EventName:     chaincodeEventData.GetEventName(),
				Payload:       chaincodeEventData.GetPayload(),
			}
			chaincodeEvents = append(chaincodeEvents, chaincodeEvent)

		}
	}

	return chaincodeEvents, nil

}
