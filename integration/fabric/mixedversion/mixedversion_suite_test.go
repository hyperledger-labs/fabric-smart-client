/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mixedversion_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
)

func TestEndToEnd(t *testing.T) { //nolint:paralleltest
	RegisterFailHandler(Fail)
	RunSpecs(t, "Mixed Version Suite")
}

var _ = AfterSuite(integration.CleanupBuildArtifacts)
