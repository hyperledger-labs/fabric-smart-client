/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package eip712 implements the EIP-712 signing envelope used to authorize IOU contract calls.
//
// It provides the domain separator (bound to the chain ID and the IOU contract address), the
// final digest over a caller-supplied struct hash, and the secp256k1 signer/verifier. The digest
// uses Keccak256 and must match the Solidity contract's recomputation exactly.
//
// Copied from Panurus x/token/services/network/evm/eip712 at commit f144b2b7f, with the
// StateDelta-specific hashStruct removed: callers hash their own typed structs (see the IOU
// Create/Update structs in examples/evmiou/iou/contract) using the
// Uint64Word/AddressWord/LeftPad helpers in encode.go and pass the result to Digest.
package eip712
