/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package atsa_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
)

func TestEndToEnd(t *testing.T) { //nolint:paralleltest
	RegisterFailHandler(Fail)
	RunSpecs(t, "Fabric-X Asset Transfer Secured Agreement")
}

var _ = AfterSuite(integration.CleanupBuildArtifacts)
