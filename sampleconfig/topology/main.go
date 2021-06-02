/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package main

import (
	"io/ioutil"

	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/chaincode"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/nochaincode"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou"
	"github.com/hyperledger-labs/fabric-smart-client/integration/generic/pingpong"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

func main() {
	gomega.RegisterFailHandler(ginkgo.Fail)
	topologies := map[string][]nwo.Topology{}

	topologies["fabric_atsa_chaincode.yaml"] = chaincode.Topology()
	topologies["fabric_atsa_nochaincode.yaml"] = nochaincode.Topology()
	topologies["fabric_iou.yaml"] = iou.Topology()

	topologies["generic_pingpong.yaml"] = pingpong.Topology()

	for name, topologies := range topologies {
		t := nwo.Topologies{Topologies: topologies}
		raw, err := t.Export()
		if err != nil {
			panic(err)
		}
		if err := ioutil.WriteFile(name, raw, 0770); err != nil {
			panic(err)
		}
	}
}
