/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"io/ioutil"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/chaincode"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/atsa/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/fpc/echo"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/twonets"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/weaver/relay"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/orion/cars"
)

func main() {
	gomega.RegisterFailHandler(ginkgo.Fail)
	topologies := map[string][]api.Topology{}

	topologies["fabric_atsa_chaincode.yaml"] = chaincode.Topology()
	topologies["fabric_atsa_nochaincode.yaml"] = fsc.Topology()
	topologies["fabric_iou.yaml"] = iou.Topology()
	topologies["fabric_twonets.yaml"] = twonets.Topology()
	topologies["fabric_fpc_echo.yaml"] = echo.Topology()
	topologies["fabric_weaver_relay.yaml"] = relay.Topology()

	topologies["fsc_pingpong.yaml"] = pingpong.Topology()

	topologies["orion_cars.yaml"] = cars.Topology()

	for name, topologies := range topologies {
		t := api.Topologies{Topologies: topologies}
		raw, err := t.Export()
		if err != nil {
			panic(err)
		}
		if err := ioutil.WriteFile(name, raw, 0770); err != nil {
			panic(err)
		}
	}
}
