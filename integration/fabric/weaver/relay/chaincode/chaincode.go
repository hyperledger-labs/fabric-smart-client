/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"log"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

type SmartContract struct {
	contractapi.Contract
}

func (s *SmartContract) Put(ctx contractapi.TransactionContextInterface, key string, value []byte) error {
	return ctx.GetStub().PutState(key, value)
}

func (s *SmartContract) Get(ctx contractapi.TransactionContextInterface, key string) ([]byte, error) {
	return ctx.GetStub().GetState(key)
}

func main() {
	chaincode, err := contractapi.NewChaincode(new(SmartContract))
	if err != nil {
		log.Panicf("Error create chaincode: %v", err)
	}

	if err := chaincode.Start(); err != nil {
		log.Panicf("Error starting asset chaincode: %v", err)
	}
}
