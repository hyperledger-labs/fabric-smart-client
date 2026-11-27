/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package crypto provides the Keccak256 hashing primitive used by the EVM client and the EIP-712
// signer: it backs on-chain object keys and the EIP-712 digest, so anything the Solidity contracts
// recompute must use it.
//
// This package deliberately does not depend on go-ethereum: its license is a hard blocker for
// the project. Keccak comes from golang.org/x/crypto/sha3.
//
// Copied from Panurus x/token/services/network/evm/crypto at commit f144b2b7f (SHA256, used there
// to match the Token SDK's token-request/public-parameters hashes, is dropped: this demo has no
// such hashes to match).
package crypto

import (
	"golang.org/x/crypto/sha3"
)

// HashLength is the length in bytes of a Keccak256 digest.
const HashLength = 32

// Keccak256 returns the Keccak-256 digest of the concatenation of data.
//
// This is the pre-standardization Keccak used by Ethereum, not FIPS-202 SHA3-256; the two
// produce different digests for the same input.
func Keccak256(data ...[]byte) []byte {
	h := sha3.NewLegacyKeccak256()
	for _, b := range data {
		// hash.Hash.Write is documented never to return an error.
		_, _ = h.Write(b)
	}

	return h.Sum(nil)
}

// Keccak256Hash returns the Keccak-256 digest of the concatenation of data as a fixed-size array.
func Keccak256Hash(data ...[]byte) [HashLength]byte {
	var out [HashLength]byte
	copy(out[:], Keccak256(data...))

	return out
}
