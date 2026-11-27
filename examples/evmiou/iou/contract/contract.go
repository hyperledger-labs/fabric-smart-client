/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package contract provides the IOU contract's compiled bytecode and the hand-written ABI
// encoding for its five calls (deploy, create, update, get), plus the EIP-712 struct hashing for
// Create and Update. It has no ABI-JSON parsing and no general ABI encoder: the contract exposes
// exactly these five shapes, and that is all this package encodes or decodes.
//
// The struct hashing here must match IOU.sol's hashStruct computation exactly; the integration
// suite proves that on a real node.
package contract

import (
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"math/big"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/client"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/crypto"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/eip712"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// wordLen is the size in bytes of an ABI word (a 256-bit slot).
const wordLen = 32

//go:embed IOU.bin
var bytecodeHex string

// bytecode is the compiled contract-creation bytecode, decoded once from the embedded hex. A
// decode failure here means IOU.bin is corrupt or missing its regeneration step, which is a build
// error, not a condition any caller can recover from.
var bytecode = mustDecodeHex(bytecodeHex)

func mustDecodeHex(s string) []byte {
	b, err := hex.DecodeString(strings.TrimSpace(s))
	if err != nil {
		panic("contract: IOU.bin does not decode as hex: " + err.Error())
	}

	return b
}

var (
	createTypeHash = crypto.Keccak256([]byte("Create(bytes32 id,uint256 amount,address borrower,address lender)"))
	updateTypeHash = crypto.Keccak256([]byte("Update(bytes32 id,uint256 amount,uint256 version)"))

	createSelector = methodID("create(bytes32,uint256,address,address,bytes,bytes,bytes)")
	updateSelector = methodID("update(bytes32,uint256,uint256,bytes,bytes,bytes)")
	getSelector    = methodID("get(bytes32)")
)

// methodID returns the 4-byte function selector for the canonical signature, e.g.
// "get(bytes32)": no argument names, no spaces, canonical type names.
func methodID(signature string) []byte {
	return crypto.Keccak256([]byte(signature))[:4]
}

// Create is the typed struct the borrower, the lender and an approver sign (EIP-712) to authorize
// a create call.
type Create struct {
	ID               [32]byte
	Amount           uint64
	Borrower, Lender client.Address
}

// HashStruct returns EIP-712's hashStruct(Create):
// keccak256(typeHash || id || amount || borrower || lender), matching IOU.sol exactly.
func (c Create) HashStruct() [32]byte {
	buf := make([]byte, 0, 4*wordLen)
	buf = append(buf, createTypeHash...)
	buf = append(buf, c.ID[:]...)
	buf = append(buf, eip712.Uint64Word(c.Amount)...)
	buf = append(buf, eip712.AddressWord(c.Borrower)...)
	buf = append(buf, eip712.AddressWord(c.Lender)...)

	return crypto.Keccak256Hash(buf)
}

// Update is the typed struct the borrower, the lender and an approver sign (EIP-712) to authorize
// an update call.
type Update struct {
	ID              [32]byte
	Amount, Version uint64
}

// HashStruct returns EIP-712's hashStruct(Update):
// keccak256(typeHash || id || amount || version), matching IOU.sol exactly.
func (u Update) HashStruct() [32]byte {
	buf := make([]byte, 0, 3*wordLen)
	buf = append(buf, updateTypeHash...)
	buf = append(buf, u.ID[:]...)
	buf = append(buf, eip712.Uint64Word(u.Amount)...)
	buf = append(buf, eip712.Uint64Word(u.Version)...)

	return crypto.Keccak256Hash(buf)
}

// IOU is the on-chain state returned by get. Version 0 means the id does not exist.
type IOU struct {
	Amount           uint64
	Borrower, Lender client.Address
	Version          uint64
}

// Domain returns the EIP-712 domain the IOU contract signs over: name "FSC IOU", version "1",
// bound to chainID and the deployed contract's address.
func Domain(chainID *big.Int, addr client.Address) eip712.Domain {
	return eip712.Domain{Name: "FSC IOU", Version: "1", ChainID: chainID, VerifyingContract: addr}
}

// DeployData returns the contract-creation calldata: the compiled bytecode followed by the
// ABI-encoded constructor argument, the approver address set.
func DeployData(approvers []client.Address) []byte {
	out := make([]byte, 0, len(bytecode)+2*wordLen+len(approvers)*wordLen)
	out = append(out, bytecode...)
	out = append(out, eip712.Uint64Word(wordLen)...)                // offset to the tail
	out = append(out, eip712.Uint64Word(uint64(len(approvers)))...) // array length
	for _, a := range approvers {
		out = append(out, eip712.AddressWord(a)...)
	}

	return out
}

// EncodeCreate encodes a call to create(bytes32,uint256,address,address,bytes,bytes,bytes).
func EncodeCreate(c Create, sigBorrower, sigLender, sigApprover []byte) []byte {
	heads := [][]byte{c.ID[:], eip712.Uint64Word(c.Amount), eip712.AddressWord(c.Borrower), eip712.AddressWord(c.Lender)}

	return encodeCall(createSelector, heads, [][]byte{sigBorrower, sigLender, sigApprover})
}

// EncodeUpdate encodes a call to update(bytes32,uint256,uint256,bytes,bytes,bytes).
func EncodeUpdate(u Update, sigBorrower, sigLender, sigApprover []byte) []byte {
	heads := [][]byte{u.ID[:], eip712.Uint64Word(u.Amount), eip712.Uint64Word(u.Version)}

	return encodeCall(updateSelector, heads, [][]byte{sigBorrower, sigLender, sigApprover})
}

// EncodeGet encodes a call to get(bytes32).
func EncodeGet(id [32]byte) []byte {
	out := make([]byte, 0, len(getSelector)+wordLen)
	out = append(out, getSelector...)
	out = append(out, id[:]...)

	return out
}

// DecodeGet decodes the return of get(bytes32): four static words, amount, borrower, lender,
// version, in that order.
func DecodeGet(ret []byte) (IOU, error) {
	if len(ret) < 4*wordLen {
		return IOU{}, errors.Errorf("contract: get return too short: got %d bytes, want at least %d", len(ret), 4*wordLen)
	}

	amount, err := decodeUint64(ret[0*wordLen : 1*wordLen])
	if err != nil {
		return IOU{}, errors.Wrapf(err, "contract: decoding amount")
	}
	version, err := decodeUint64(ret[3*wordLen : 4*wordLen])
	if err != nil {
		return IOU{}, errors.Wrapf(err, "contract: decoding version")
	}

	return IOU{
		Amount:   amount,
		Borrower: client.BytesToAddress(ret[1*wordLen : 2*wordLen]),
		Lender:   client.BytesToAddress(ret[2*wordLen : 3*wordLen]),
		Version:  version,
	}, nil
}

// decodeUint64 decodes a right-aligned uint64 word, erroring if the high 24 bytes are set rather
// than silently truncating a value that does not fit.
func decodeUint64(word []byte) (uint64, error) {
	for _, b := range word[:wordLen-8] {
		if b != 0 {
			return 0, errors.New("contract: value does not fit in uint64")
		}
	}

	return binary.BigEndian.Uint64(word[wordLen-8:]), nil
}

// encodeCall lays out calldata for a method whose arguments are, in declaration order, all static
// words followed by all dynamic `bytes` arguments (the shape of both create and update): the
// selector, then the static words and one offset word per dynamic argument (the head), then each
// dynamic argument's length-prefixed, zero-padded payload (the tail).
func encodeCall(selector []byte, staticWords, dynamicData [][]byte) []byte {
	headSize := (len(staticWords) + len(dynamicData)) * wordLen

	head := make([]byte, 0, headSize)
	for _, w := range staticWords {
		head = append(head, w...)
	}

	var tail []byte
	for _, d := range dynamicData {
		head = append(head, eip712.Uint64Word(uint64(headSize+len(tail)))...)
		tail = append(tail, encodeBytesTail(d)...)
	}

	out := make([]byte, 0, len(selector)+len(head)+len(tail))
	out = append(out, selector...)
	out = append(out, head...)
	out = append(out, tail...)

	return out
}

// encodeBytesTail encodes a dynamic `bytes` value's tail payload: a length word, then the data
// right-padded with zeros to a multiple of 32 bytes.
func encodeBytesTail(b []byte) []byte {
	pad := (wordLen - len(b)%wordLen) % wordLen
	out := make([]byte, 0, wordLen+len(b)+pad)
	out = append(out, eip712.Uint64Word(uint64(len(b)))...)
	out = append(out, b...)
	out = append(out, make([]byte, pad)...)

	return out
}
