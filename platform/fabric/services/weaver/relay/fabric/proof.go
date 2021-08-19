/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

// Proof models a Relay proof for a Fabric Query
type Proof struct {
	B64ViewProto string
	Address      string
}
