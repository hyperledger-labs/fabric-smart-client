/*
SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"fmt"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

// SmartContract provides functions for Event Listening
type SmartContract struct {
	contractapi.Contract
}

func (s *SmartContract) InitLedger(ctx contractapi.TransactionContextInterface) {
	fmt.Println("Init Function Invoked")
}

func (s *SmartContract) CreateAsset(ctx contractapi.TransactionContextInterface) {
	ctx.GetStub().SetEvent("CreateAsset", []byte("Invoked Create Asset Successfully"))

}

func (s *SmartContract) UpdateAsset(ctx contractapi.TransactionContextInterface) {
	ctx.GetStub().SetEvent("UpdateAsset", []byte("Invoked Update Asset Successfully"))
}
