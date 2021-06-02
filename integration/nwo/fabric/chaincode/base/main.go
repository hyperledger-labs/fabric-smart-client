/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"fmt"
	"os"

	"github.com/hyperledger/fabric-chaincode-go/shim"
	pb "github.com/hyperledger/fabric-protos-go/peer"
)

type Chaincode struct{}

func (t *Chaincode) Init(stub shim.ChaincodeStubInterface) pb.Response {
	fmt.Println("Init...")
	return shim.Success(nil)
}

func (t *Chaincode) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	function, params := stub.GetFunctionAndParameters()
	fmt.Printf("Invoke function %s with params %v\n", function, params)
	return shim.Success(nil)
}

func main() {
	err := shim.Start(&Chaincode{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Exiting chaincode: %s", err)
		os.Exit(2)
	}
}
