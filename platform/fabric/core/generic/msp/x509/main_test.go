/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package x509

import (
	"os"
	"testing"

	fabricmsp "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp"
)

func TestMain(m *testing.M) {
	// Fabric's BCCSP factory is initialized through a global sync.Once.
	// Prime it with a valid test MSP so negative-path tests do not poison
	// global state for the rest of the package when tests run in parallel.
	if _, err := fabricmsp.GetLocalMspConfig("./testdata/msp", nil, "apple"); err != nil {
		panic(err)
	}

	os.Exit(m.Run())
}
