package main

import (
	"encoding/json"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou/views"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/client/web"
)

func main() {
	webClientConfig, err := web.NewConfigFromFSC("../nodes/approver")
	must(err)

	webClient, err := web.NewClient(webClientConfig)
	must(err)
	payload, err := json.Marshal(&views.Create{
		Amount: 10,
	})
	must(err)

	res, err := webClient.CallView("create", payload)
	must(err)

	fmt.Printf("%v\n", res)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
