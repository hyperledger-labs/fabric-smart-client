/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package evm is the nwo platform for the EVM IOU demo. During GenerateArtifacts it either starts
// a Besu development node or validates an externally supplied EVM endpoint, generates a secp256k1
// key per FSC node, funds the submitter nodes, deploys the IOU contract, and renders each FSC
// node's "evm" core.yaml extension.
//
// Besu support (besu.go) and key generation (keys.go, including DevFundedKey) are adapted from
// Panurus (x/token/services/network/evm) at commit f144b2b7f.
package evm
