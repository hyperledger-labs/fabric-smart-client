/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package eip712

import (
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/crypto"
)

// Digest returns the EIP-712 signing digest for a typed struct, given its hashStruct value:
//
//	keccak256(0x19 || 0x01 || domainSeparator || structHash)
//
// Signers sign this digest with their secp256k1 key, and the contract recomputes it identically
// from the typed struct before verifying the signatures. structHash is the caller's own
// hashStruct(...) over its typed struct (see the IOU Create/Update structs in
// examples/evmiou/iou/contract), computed with the Uint64Word/AddressWord/LeftPad helpers
// in encode.go.
func Digest(d Domain, structHash [32]byte) [32]byte {
	sep := d.Separator()

	buf := make([]byte, 0, 2+2*wordLen)
	buf = append(buf, 0x19, 0x01)
	buf = append(buf, sep[:]...)
	buf = append(buf, structHash[:]...)

	return crypto.Keccak256Hash(buf)
}
