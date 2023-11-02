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
	Err           error
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
	payl, err := protoutil.UnmarshalPayload(env.Payload)
	if err != nil {
		return nil, err
	}
	tx, err := protoutil.UnmarshalTransaction(payl.Data)
	if err != nil {
		return nil, err
	}
	if len(tx.Actions) == 0 {
		return nil, nil
	}
	_, chaincodeAction, err := protoutil.GetPayloads(tx.Actions[0])
	if err != nil {
		return nil, err
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
