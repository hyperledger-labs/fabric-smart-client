/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"github.com/pkg/errors"

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

// ChaincodeEvent models the chaincode event details.
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

// validChaincodeEvent validates the chaincode event received.
func validChaincodeEvent(event *peer.ChaincodeEvent) bool {
	return event != nil && len(event.GetChaincodeId()) > 0 && len(event.GetEventName()) > 0 && len(event.GetTxId()) > 0
}

// readChaincodeEvent parses the envelope proto message to get the chaincode events.
func readChaincodeEvent(env *common.Envelope, blockNumber uint64) (*ChaincodeEvent, error) {
	var chaincodeEvent *ChaincodeEvent
	chaincodeAction, err := protoutil.GetActionFromEnvelopeMsg(env)
	if err != nil {
		return nil, errors.Wrapf(err, "Error getting chaincode actions from envelope")
	}

	if chaincodeAction == nil {
		return nil, nil
	}

	chaincodeEventData, err := protoutil.UnmarshalChaincodeEvents(chaincodeAction.GetEvents())
	if err != nil {
		return nil, errors.Wrapf(err, "Error getting chaincode event from chaincode actions")
	}

	if !validChaincodeEvent(chaincodeEventData) {
		return nil, nil
	}
	chaincodeEvent = &ChaincodeEvent{
		BlockNumber:   blockNumber,
		TransactionID: chaincodeEventData.GetTxId(),
		ChaincodeID:   chaincodeEventData.GetChaincodeId(),
		EventName:     chaincodeEventData.GetEventName(),
		Payload:       chaincodeEventData.GetPayload(),
	}

	return chaincodeEvent, nil

}
