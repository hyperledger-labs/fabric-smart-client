/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"encoding/base64"
	"log"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
	"github.com/pkg/errors"
)

type PvtChaincodeValidator struct {
	contractapi.Contract
}

func (cc *PvtChaincodeValidator) Store(ctx contractapi.TransactionContextInterface, id string, raw string) error {
	if len(id) == 0 {
		return errors.New("id must be non-empty")
	}
	if len(raw) == 0 {
		return errors.New("raw must be non-empty")
	}

	decodedID, err := base64.StdEncoding.DecodeString(id)
	if err != nil {
		return errors.Errorf("failed to decode [%s]", id)
	}

	if err := ctx.GetStub().PutState(string(decodedID), []byte(raw)); err != nil {
		return errors.Wrapf(err, "failed to store key-pair with id [%s]", id)
	}

	return nil
}

func main() {
	chaincode, err := contractapi.NewChaincode(new(PvtChaincodeValidator))
	if err != nil {
		log.Panicf("Error create transfer asset chaincode: %v", err)
	}
	if err := chaincode.Start(); err != nil {
		log.Panicf("Error starting asset chaincode: %v", err)
	}
}
