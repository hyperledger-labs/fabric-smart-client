/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package bridge

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/csp/idemix/crypto"
	"github.com/hyperledger/fabric-amcl/amcl"
)

// NewRandOrPanic return a new amcl PRG or panic
func NewRandOrPanic() *amcl.RAND {
	rng, err := crypto.GetRand()
	if err != nil {
		panic(err)
	}
	return rng
}
