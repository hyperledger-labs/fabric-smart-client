/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package eip712

import (
	"math/big"

	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/client"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/crypto"
)

// domainTypeHash = keccak256("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)").
var domainTypeHash = crypto.Keccak256([]byte("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"))

// Domain binds a signature to a specific chain and contract, so a signature cannot be replayed on
// another chain or against another contract. The IOU demo uses Name "FSC IOU", Version "1", the
// chain ID of the target network and the deployed IOU contract's address.
type Domain struct {
	Name              string
	Version           string
	ChainID           *big.Int
	VerifyingContract client.Address
}

// Separator returns the EIP-712 domain separator:
// keccak256(domainTypeHash || keccak256(name) || keccak256(version) || chainId || verifyingContract).
//
// A nil ChainID is treated as zero rather than panicking; the caller validates that the chain ID is
// set, and a zero here yields a separator that will not match the contract, surfacing the
// misconfiguration as a signature-verification failure rather than a crash during signing.
func (d Domain) Separator() [32]byte {
	chainID := d.ChainID
	if chainID == nil {
		chainID = big.NewInt(0)
	}

	buf := make([]byte, 0, 5*wordLen)
	buf = append(buf, domainTypeHash...)
	buf = append(buf, crypto.Keccak256([]byte(d.Name))...)
	buf = append(buf, crypto.Keccak256([]byte(d.Version))...)
	buf = append(buf, bigWord(chainID)...)
	buf = append(buf, AddressWord(d.VerifyingContract)...)

	return crypto.Keccak256Hash(buf)
}
