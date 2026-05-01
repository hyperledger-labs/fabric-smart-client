package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/hyperledger-labs/fabric-smart-client/examples/chaincode-to-fsc/internal/flow"
)

func main() {
	assetID := flag.String("asset-id", "", "asset identifier")
	newOwner := flag.String("new-owner", "", "new owner")
	flag.Parse()

	if *assetID == "" || *newOwner == "" {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/token-transfer --asset-id <id> --new-owner <owner>")
		os.Exit(2)
	}

	result, err := flow.TransferAsset(*assetID, *newOwner)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(result)
}