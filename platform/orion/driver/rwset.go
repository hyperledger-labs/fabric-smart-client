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

type RWSet interface {
	IsValid() error

	Clear(ns string) error

	// SetState sets the given value for the given namespace and key.
	SetState(namespace string, key string, value []byte) error

	GetState(namespace string, key string, opts ...GetStateOpt) ([]byte, error)

	// DeleteState deletes the given namespace and key
	DeleteState(namespace string, key string) error

	GetStateMetadata(namespace, key string, opts ...GetStateOpt) (map[string][]byte, error)

	// SetStateMetadata sets the metadata associated with an existing key-tuple <namespace, key>
	SetStateMetadata(namespace, key string, metadata map[string][]byte) error

	GetReadKeyAt(ns string, i int) (string, error)

	// GetReadAt returns the i-th read (key, value) in the namespace ns  of this rwset.
	// The value is loaded from the ledger, if present. If the key's version in the ledger
	// does not match the key's version in the read, then it returns an error.
	GetReadAt(ns string, i int) (string, []byte, error)

	// GetWriteAt returns the i-th write (key, value) in the namespace ns of this rwset.
	GetWriteAt(ns string, i int) (string, []byte, error)

	// NumReads returns the number of reads in the namespace ns  of this rwset.
	NumReads(ns string) int

	// NumWrites returns the number of writes in the namespace ns of this rwset.
	NumWrites(ns string) int

	// Namespaces returns the namespace labels in this rwset.
	Namespaces() []string

	AppendRWSet(raw []byte, nss ...string) error

	Bytes() ([]byte, error)

	Done()

	Equals(rws interface{}, nss ...string) error
}
