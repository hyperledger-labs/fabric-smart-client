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

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
)

type CC struct {
}

func (cc *CC) Init(stub shim.ChaincodeStubInterface) pb.Response {
	fmt.Println("Init...")

	return shim.Success(nil)
}

func (cc *CC) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	fn, _ := stub.GetFunctionAndParameters()

	fmt.Printf("Invoke function [%s]...\n", fn)
	switch fn {
	case state.CertificationFnc:
		args := stub.GetStringArgs()
		fmt.Printf("Invoke function [%s] with args [%v]\n", fn, args)
		if len(args) != 3 {
			fmt.Printf("Invalid number of arguments, expected two\n")
			return shim.Error("Invalid number of arguments, expected two")
		}

		id := args[1]
		v, err := stub.GetState(id)
		if err != nil {
			fmt.Printf("failed getting state [%s]: %s\n", id, err)
			return shim.Error(fmt.Sprintf("failed getting state [%s]: %s", id, err))
		}
		if err := stub.PutState(id, v); err != nil {
			fmt.Printf("failed setting state [%s]: %s\n", id, err)
			return shim.Error(fmt.Sprintf("failed setting state [%s]: %s", id, err))
		}
	default:
		return shim.Error(fmt.Sprintf("function not recognized [%s]", fn))
	}

	return shim.Success(nil)
}

func main() {
	err := shim.Start(&CC{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Exiting chaincode: %s", err)
		os.Exit(2)
	}
}
