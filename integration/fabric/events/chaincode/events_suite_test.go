/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"testing"
)

func TestEndToEnd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Events (With Chaincode)")
}

func StartPort() int {
	return integration.AssetTransferEventsAgreementWithChaincode.StartPortForNode()
}
