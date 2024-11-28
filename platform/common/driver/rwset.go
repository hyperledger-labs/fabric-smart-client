/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

type GetStateOpt int

const (
	FromStorage GetStateOpt = iota
	FromIntermediate
	FromBoth
)

// RWSet models a namespaced versioned read-write set
type RWSet interface {
	// IsValid returns true if this rwset is valid.
	// A rwset is valid if:
	// 1. It exists in the vault as valid
	// 2. All the read dependencies are satisfied by the vault
	IsValid() error

	IsClosed() bool

	// Clear remove the passed namespace from this rwset
	Clear(ns Namespace) error

	// AddReadAt adds a read dependency for the given namespace and key at the given version
	AddReadAt(ns Namespace, key string, version RawVersion) error

	// SetState sets the given value for the given namespace and key.
	SetState(namespace Namespace, key PKey, value RawValue) error

	GetState(namespace Namespace, key PKey, opts ...GetStateOpt) (RawValue, error)

	// GetDirectState accesses the state using the query executor without looking into the RWSet.
	// This way we can access the query executor while we have a RWSet already open avoiding nested RLocks.
	GetDirectState(namespace Namespace, key PKey) (RawValue, error)

	// DeleteState deletes the given namespace and key
	DeleteState(namespace Namespace, key PKey) error

	GetStateMetadata(namespace Namespace, key PKey, opts ...GetStateOpt) (Metadata, error)

	// SetStateMetadata sets the metadata associated with an existing key-tuple <namespace, key>
	SetStateMetadata(namespace Namespace, key PKey, metadata Metadata) error

	GetReadKeyAt(ns Namespace, i int) (PKey, error)

	// GetReadAt returns the i-th read (key, value) in the namespace ns  of this rwset.
	// The value is loaded from the ledger, if present. If the key's version in the ledger
	// does not match the key's version in the read, then it returns an error.
	GetReadAt(ns Namespace, i int) (PKey, RawValue, error)

	// GetWriteAt returns the i-th write (key, value) in the namespace ns of this rwset.
	GetWriteAt(ns Namespace, i int) (PKey, RawValue, error)

	// NumReads returns the number of reads in the namespace ns  of this rwset.
	NumReads(ns Namespace) int

	// NumWrites returns the number of writes in the namespace ns of this rwset.
	NumWrites(ns Namespace) int

	// Namespaces returns the namespace labels in this rwset.
	Namespaces() []Namespace

	AppendRWSet(raw []byte, nss ...Namespace) error

	Bytes() ([]byte, error)

	Done()

	Equals(rws interface{}, nss ...Namespace) error
}
