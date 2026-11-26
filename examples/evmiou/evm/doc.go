/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package evm is the root of a go-ethereum-free EVM client, copied for the FSC
// IOU-on-EVM demo (Besu and Hiero).
//
// It has three subpackages: client (the JSON-RPC client and EIP-1559
// transaction signing), crypto (Keccak256) and eip712 (the EIP-712 domain,
// digest and secp256k1 signer used to authorize contract calls). Nothing
// here depends on go-ethereum, whose license is a hard blocker for this
// project; the Ethereum primitives are reimplemented locally.
//
// The code is copied from Panurus (x/token/services/network/evm) at commit
// f144b2b7f, with the statedelta-specific parts removed: this demo signs its
// own typed structs rather than a Token SDK StateDelta.
package evm
