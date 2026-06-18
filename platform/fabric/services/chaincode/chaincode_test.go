/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"testing"
)

// We can test the With* wrappers even with a nil *fabric.ChaincodeEndorse
// However, since it calls s.che.With... it will panic if che is nil.
// Since fabric.ChaincodeEndorse is a concrete struct, we can't easily initialize it
// without the full fabric network.
// If it panics on nil, we can't test it this way. Let's see if we can create a dummy fabric.ChaincodeEndorse.
// Wait, fabric.ChaincodeEndorse has no exported fields. So we can't even inject a dummy easily.
// Let's at least test the interfaces to ensure they compile and have the expected methods.

func TestChaincodeInterfaces(t *testing.T) {
	t.Parallel()

	// This is a compilation test to ensure the interfaces match
	var _ Endorse = (*stdEndorse)(nil)
	var _ Query = (*stdQuery)(nil)
	var _ Chaincode = (*stdChaincode)(nil)
}
