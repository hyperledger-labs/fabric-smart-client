/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"log"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
	"github.com/pkg/errors"
)

type SmartContract struct {
	contractapi.Contract
}

func (s *SmartContract) Put(ctx contractapi.TransactionContextInterface, key string, value string) error {
	return ctx.GetStub().PutState(key, []byte(value))
}

func (s *SmartContract) Get(ctx contractapi.TransactionContextInterface, key string) (string, error) {
	v, err := ctx.GetStub().GetState(key)
	if err != nil {
		return "", errors.Wrapf(err, "failed getting state [%s]", key)
	}
	err = ctx.GetStub().PutState(key, v)
	if err != nil {
		return "", errors.Wrapf(err, "failed putting state [%s:%s]", key, string(v))
	}
	if len(v) == 0 {
		return "", nil
	}
	return string(v), nil
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
