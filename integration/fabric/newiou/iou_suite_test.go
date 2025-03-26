/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package newiou_test

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestEndToEnd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "NewIOU Suite")
}

func StartPort() int {
	return integration.NewIOUPort.StartPortForNode()
}
