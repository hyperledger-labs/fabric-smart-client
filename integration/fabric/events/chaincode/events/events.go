/*
SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"fmt"

	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

// SmartContract provides functions for Event Listening
type SmartContract struct {
	contractapi.Contract
}

func (*SmartContract) InitLedger(_ contractapi.TransactionContextInterface) {
	fmt.Println("Init Function Invoked")
}

func (*SmartContract) CreateAsset(ctx contractapi.TransactionContextInterface) error {
	return ctx.GetStub().SetEvent("CreateAsset", []byte("Invoked Create Asset Successfully"))
}

func (*SmartContract) UpdateAsset(ctx contractapi.TransactionContextInterface) error {
	return ctx.GetStub().SetEvent("UpdateAsset", []byte("Invoked Update Asset Successfully"))
}
