/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package translog

import (
	"golang.org/x/mod/sumdb/tlog"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// Tree is an append-only Merkle tree over log entry leaves, hashed and
// shaped the same way as Certificate Transparency (RFC 6962): the scheme
// golang.org/x/mod/sumdb/tlog implements and transparency-dev/merkle/proof
// verifies against. The zero value is an empty tree.
type Tree struct {
	// hashes holds every hash tlog.StoredHashes has produced on Append, in
	// tlog.StoredHashIndex order. It is everything TreeHash, ProveRecord and
	// ProveTree need to recompute roots and proofs for any size <= Size().
	hashes []tlog.Hash
	size   uint64
}

// NewTree builds a tree containing leaves, in order.
func NewTree(leaves [][]byte) (*Tree, error) {
	t := &Tree{}
	for _, leaf := range leaves {
		if err := t.Append(leaf); err != nil {
			return nil, err
		}
	}
	return t, nil
}

// Append adds leaf as the next record in the tree.
func (t *Tree) Append(leaf []byte) error {
	hashes, err := tlog.StoredHashes(int64(t.size), leaf, t.reader())
	if err != nil {
		return errors.Wrap(err, "computing stored hashes")
	}
	t.hashes = append(t.hashes, hashes...)
	t.size++
	return nil
}

// Size returns the number of leaves appended so far.
func (t *Tree) Size() uint64 {
	return t.size
}

// Root returns the root hash of the tree truncated to its first size
// leaves. size must be at most Size().
func (t *Tree) Root(size uint64) ([]byte, error) {
	if size > t.size {
		return nil, errors.Errorf("size %d is beyond the tree size %d", size, t.size)
	}
	h, err := tlog.TreeHash(int64(size), t.reader())
	if err != nil {
		return nil, errors.Wrap(err, "computing root hash")
	}
	return h[:], nil
}

// InclusionProof proves that the leaf at index is included in the tree
// truncated to its first size leaves. Requires index < size <= Size().
func (t *Tree) InclusionProof(index, size uint64) ([][]byte, error) {
	if size > t.size {
		return nil, errors.Errorf("size %d is beyond the tree size %d", size, t.size)
	}
	if index >= size {
		return nil, errors.Errorf("index %d is beyond size %d", index, size)
	}
	p, err := tlog.ProveRecord(int64(size), int64(index), t.reader())
	if err != nil {
		return nil, errors.Wrap(err, "computing inclusion proof")
	}
	return hashesToBytes(p), nil
}

// ConsistencyProof proves that the tree truncated to newSize leaves extends
// the tree truncated to oldSize leaves. Requires oldSize <= newSize <=
// Size().
func (t *Tree) ConsistencyProof(oldSize, newSize uint64) ([][]byte, error) {
	if newSize > t.size {
		return nil, errors.Errorf("size %d is beyond the tree size %d", newSize, t.size)
	}
	if oldSize > newSize {
		return nil, errors.Errorf("old size %d is beyond new size %d", oldSize, newSize)
	}
	if oldSize == 0 {
		// Any size is consistent with the empty tree, with an empty proof;
		// see transparency-dev/merkle/proof.VerifyConsistency. tlog.ProveTree
		// itself rejects a size of 0.
		return [][]byte{}, nil
	}
	p, err := tlog.ProveTree(int64(newSize), int64(oldSize), t.reader())
	if err != nil {
		return nil, errors.Wrap(err, "computing consistency proof")
	}
	return hashesToBytes(p), nil
}

// reader lets tlog read back the hashes Append has stored so far.
func (t *Tree) reader() tlog.HashReaderFunc {
	return func(indexes []int64) ([]tlog.Hash, error) {
		hashes := make([]tlog.Hash, len(indexes))
		for i, index := range indexes {
			if index < 0 || index >= int64(len(t.hashes)) {
				return nil, errors.Errorf("stored hash index %d out of range", index)
			}
			hashes[i] = t.hashes[index]
		}
		return hashes, nil
	}
}

// hashesToBytes converts tlog hashes to the [][]byte proof shape that
// transparency-dev/merkle/proof verifies against.
func hashesToBytes(hashes []tlog.Hash) [][]byte {
	out := make([][]byte, len(hashes))
	for i, h := range hashes {
		out[i] = h[:]
	}
	return out
}
