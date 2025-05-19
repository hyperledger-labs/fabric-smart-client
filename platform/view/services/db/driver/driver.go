/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/pkg/errors"
)

var (
	// UniqueKeyViolation happens when we try to insert a record with a conflicting unique key (e.g. replicas)
	UniqueKeyViolation = errors.New("unique key violation")
	// DeadlockDetected happens when two transactions are taking place at the same time and interact with the same rows
	DeadlockDetected = errors.New("deadlock detected")
	// SqlBusy happens when two transactions are trying to write at the same time. Can be avoided by opening the database in exclusive mode
	SqlBusy = errors.New("sql is busy")
)

type SQLError = error

type UnversionedRead = driver.UnversionedRead
type UnversionedValue = driver.UnversionedValue

type QueryExecutor = driver.QueryExecutor

//go:generate counterfeiter -o sql/query/common/mock/error_wrapper.go -fake-name SQLErrorWrapper . SQLErrorWrapper

// SQLErrorWrapper transforms the different errors returned by various SQL implementations into an SQLError that is common
type SQLErrorWrapper interface {
	WrapError(error) error
}

type BindingStore = driver.BindingStore

type SignerInfoStore = driver.SignerInfoStore

type AuditInfoStore = driver.AuditInfoStore

type EndorseTxStore = driver.EndorseTxStore[string]

type MetadataStore = driver.MetadataStore[string, []byte]

type EnvelopeStore = driver.EnvelopeStore[string]

type VaultStore = driver.VaultStore

// KeyValueStore models a key-value storage place
type KeyValueStore interface {
	// SetState sets the given value for the given namespace, key, and version
	SetState(namespace driver.Namespace, key driver.PKey, value driver.UnversionedValue) error
	// SetStates sets the given values for the given namespace, key, and version
	SetStates(namespace driver.Namespace, kvs map[driver.PKey]driver.UnversionedValue) map[driver.PKey]error
	// GetState gets the value and version for given namespace and key
	GetState(namespace driver.Namespace, key driver.PKey) (driver.UnversionedValue, error)
	// DeleteState deletes the given namespace and key
	DeleteState(namespace driver.Namespace, key driver.PKey) error
	// DeleteStates deletes the given namespace and keys
	DeleteStates(namespace driver.Namespace, keys ...driver.PKey) map[driver.PKey]error
	// GetStateRangeScanIterator returns an iterator that contains all the key-values between given key ranges.
	// startKey is included in the results and endKey is excluded. An empty startKey refers to the first available key
	// and an empty endKey refers to the last available key. For scanning all the keys, both the startKey and the endKey
	// can be supplied as empty strings. However, a full scan should be used judiciously for performance reasons.
	GetStateRangeScanIterator(namespace driver.Namespace, startKey, endKey driver.PKey) (collections.Iterator[*driver.UnversionedRead], error)
	// GetStateSetIterator returns an iterator that contains all the values for the passed keys.
	// The order is not respected.
	GetStateSetIterator(ns driver.Namespace, keys ...driver.PKey) (collections.Iterator[*driver.UnversionedRead], error)
	// Close closes this persistence instance
	Close() error
	// BeginUpdate starts the session
	BeginUpdate() error
	// Commit commits the changes since BeginUpdate
	Commit() error
	// Discard discards the changes since BeginUpdate
	Discard() error
	// Stats returns driver specific statistics of the datastore
	Stats() any
}

// VersionedPersistence models a versioned key-value storage place
type VersionedPersistence = KeyValueStore

type UnversionedWriteTransaction interface {
	// SetState sets the given value for the given namespace, key
	SetState(namespace driver.Namespace, key driver.PKey, value UnversionedValue) error
	// DeleteState deletes the given namespace and key
	DeleteState(namespace driver.Namespace, key driver.PKey) error
	// Commit commits the changes since BeginUpdate
	Commit() error
	// Discard discards the changes since BeginUpdate
	Discard() error
}

// PersistenceName is the key of a persistence configuration in the core.yaml under fsc.persistences
// Each store has a property "persistence" that references to a persistence config by its name
// If the persistence property is not populated, the "default" persistence name is implied (if this is defined under fsc.persistences).
type PersistenceName string

// PersistenceConfig reads the section under fsc.persistences in the core.yaml
type PersistenceConfig interface {
	// GetDriverType returns the persistence type (memory, postgres, sqlite) of a specific persistence config
	// Corresponds to fsc.persistences.{{persistence_name}}.type
	GetDriverType(PersistenceName) (driver.PersistenceType, error)

	// UnmarshalDriverOpts retrieves the persistence options of a specific persistence config
	// The actual options may vary depending on the persistence type
	// Corresponds to fsc.persistences.{{persistence_name}}.opts
	UnmarshalDriverOpts(name PersistenceName, v any) error
}

// Config provides access to the underlying configuration
type Config interface {
	// IsSet checks to see if the key has been set in any of the data locations
	IsSet(key string) bool
	// UnmarshalKey takes the value corresponding to the passed key and unmarshals it into the passed structure
	UnmarshalKey(key string, rawVal interface{}) error
}

type NamedDriver = driver.NamedDriver[Driver]

type Driver interface {
	// NewKVS returns a new KeyValueStore for the passed data source and config
	NewKVS(PersistenceName, ...string) (KeyValueStore, error)
	// NewBinding returns a new BindingStore for the passed data source and config
	NewBinding(PersistenceName, ...string) (BindingStore, error)
	// NewSignerInfo returns a new SignerInfoStore for the passed data source and config
	NewSignerInfo(PersistenceName, ...string) (SignerInfoStore, error)
	// NewAuditInfo returns a new AuditInfoStore for the passed data source and config
	NewAuditInfo(PersistenceName, ...string) (AuditInfoStore, error)
	// NewEndorseTx returns a new EndorseTxStore for the passed data source and config
	NewEndorseTx(PersistenceName, ...string) (EndorseTxStore, error)
	// NewMetadata returns a new MetadataStore for the passed data source and config
	NewMetadata(PersistenceName, ...string) (MetadataStore, error)
	// NewEnvelope returns a new EnvelopeStore for the passed data source and config
	NewEnvelope(PersistenceName, ...string) (EnvelopeStore, error)
	// NewVault returns a new VaultStore for the passed data source and config
	NewVault(PersistenceName, ...string) (driver.VaultStore, error)
}

type (
	ColumnKey       = string
	TriggerCallback func(Operation, map[ColumnKey]string)
	Operation       int
)

const (
	Unknown Operation = iota
	Delete
	Insert
	Update
)

type Notifier interface {
	// Subscribe registers a listener for when a value is inserted/updated/deleted in the given table
	Subscribe(callback TriggerCallback) error
	// UnsubscribeAll removes all registered listeners for the given table
	UnsubscribeAll() error
}

type UnversionedNotifier interface {
	KeyValueStore
	Notifier
}
