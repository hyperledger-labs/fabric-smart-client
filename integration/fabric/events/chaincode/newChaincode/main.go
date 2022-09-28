/*
SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"log"

	chaincode "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/events/chaincode/newChaincode/events"
	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

func main() {
	eventsChaincode, err := contractapi.NewChaincode(&chaincode.SmartContract{})
	if err != nil {
		log.Panicf("Error creating events chaincode: %v", err)
	}

	if err := eventsChaincode.Start(); err != nil {
		log.Panicf("Error starting events chaincode: %v", err)
	}
}
