/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"bytes"
	"context"
	"crypto/sha256"
	"maps"
	"slices"
	"sync"
	"unsafe"

	"github.com/hyperledger/fabric-x-common/api/committerpb"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault"
	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice"
)

const (
	metaNamespace = "_meta"
)

// Vault implements the fdriver.Vault interface for FabricX. Several interface methods
// (CommitTX/DiscardTx/SetDiscarded/Match/RWSExists) exist only to satisfy that contract; FabricX
// has no local commit pipeline, so they are never invoked in the FabricX wiring and panic if
// reached (which would indicate the vault was wired into a generic committer by mistake).
type Vault struct {
	queryService queryservice.QueryService // Remote query service for state and status queries
	mds          fdriver.MetadataService   // Field-mapping (hash-hiding) metadata store
	marshaller   *Marshaller               // Marshaller for RWSet serialization
}

// NewVault creates a new Vault instance with the given QueryService and MetadataService.
// The vault uses the query service for all remote state queries and transaction status
// lookups, and the metadata service to resolve hash-hiding field mappings on a metadata miss.
//
// Parameters:
//   - qs: QueryService instance for remote queries
//   - mds: MetadataService backing the (ns,key,digest) field-mapping store (may be nil in tests)
//
// Returns:
//   - *Vault: A new vault instance ready for use
func NewVault(qs queryservice.QueryService, mds fdriver.MetadataService) *Vault {
	return &Vault{
		queryService: qs,
		mds:          mds,
		marshaller:   NewMarshaller(),
	}
}

// queryExecutor wraps the QueryService to implement the cdriver.QueryExecutor interface.
// It delegates all state queries to the remote QueryService.
type queryExecutor struct {
	qs  queryservice.QueryService // Remote query service
	ctx context.Context           // Ctx for queries
}

// GetState retrieves the state for a specific namespace and key from the remote QueryService.
// Returns nil if the key does not exist.
func (qe *queryExecutor) GetState(ctx context.Context, namespace cdriver.Namespace, key cdriver.PKey) (*cdriver.VaultRead, error) {
	vaultValue, err := qe.qs.GetState(namespace, key)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get state for namespace=%s, key=%s", namespace, key)
	}
	if vaultValue == nil {
		return nil, nil
	}
	return &cdriver.VaultRead{
		Key:     key,
		Raw:     vaultValue.Raw,
		Version: vaultValue.Version,
	}, nil
}

// GetStateMetadata retrieves metadata for a specific namespace and key.
// Since QueryService doesn't support direct metadata queries, this returns empty metadata
// and the version from the state value.
func (qe *queryExecutor) GetStateMetadata(ctx context.Context, namespace cdriver.Namespace, key cdriver.PKey) (cdriver.Metadata, cdriver.RawVersion, error) {
	// QueryService doesn't support metadata queries directly
	// Return empty metadata and version from state
	vaultValue, err := qe.qs.GetState(namespace, key)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to get state metadata for namespace=%s, key=%s", namespace, key)
	}
	if vaultValue == nil {
		return nil, nil, nil
	}
	return nil, vaultValue.Version, nil
}

// GetStateRange returns an error as range queries are not supported by the QueryService.
func (qe *queryExecutor) GetStateRange(ctx context.Context, namespace cdriver.Namespace, startKey, endKey cdriver.PKey) (cdriver.VersionedResultsIterator, error) {
	// QueryService doesn't support range queries
	return nil, errors.New("GetStateRange not supported by VaultX QueryService")
}

// Done performs cleanup for the query executor. Currently a no-op as no cleanup is needed.
func (qe *queryExecutor) Done() error {
	// No cleanup needed for query executor
	return nil
}

// NewQueryExecutor creates a new query executor that wraps the QueryService.
// The executor can be used to query state from the remote service.
func (v *Vault) NewQueryExecutor(ctx context.Context) (cdriver.QueryExecutor, error) {
	return &queryExecutor{
		qs:  v.queryService,
		ctx: ctx,
	}, nil
}

// rwSetWrapper wraps a ReadWriteSet to implement the cdriver.RWSet interface.
// It provides read/write operations with QueryService integration for state queries.
//
// Concurrency: every method is race-free — the mutex guards all shared state, so
// concurrent calls cannot corrupt the RWSet or trip the race detector. That is weaker
// than atomicity: methods that consult the RWSet, query the network, and then mutate
// (notably GetState) release the lock in between, so a concurrent SetState on the same
// key can interleave. See GetState for what that produces. Callers that need a coherent
// simulation must not mutate one RWSet from multiple goroutines.
type rwSetWrapper struct {
	txID cdriver.TxID          // Transaction ID
	rws  *vault.ReadWriteSet   // Underlying read-write set
	qe   cdriver.QueryExecutor // Query executor for state queries
	v    *Vault                // Parent vault for accessing query service

	mu sync.Mutex // Protects rws, nsVersions and cachedBytes
	// nsVersions holds the _meta version pinned for each namespace at the moment it first
	// entered the RWSet, alongside the key read versions captured by GetState. Together
	// they form one simulation snapshot.
	nsVersions  map[cdriver.Namespace]cdriver.RawVersion
	cachedBytes []byte // Memoized serialization; Bytes() is pure, so this is only an optimization
}

// pinNamespace resolves and records the version of ns unless it is already pinned.
// The first pin wins: a namespace's version is fixed at first touch and never revised,
// so repeated serializations of the same RWSet agree.
//
// The version is read from _meta at the moment the namespace enters the RWSet, exactly
// as GetState reads a key's version when it enters the read set. Nothing is cached
// between RWSets, so a namespace policy update is picked up by the next transaction.
//
// The lookup runs without holding r.mu, so a slow query service cannot stall unrelated
// operations on this RWSet.
func (r *rwSetWrapper) pinNamespace(ns cdriver.Namespace) error {
	r.mu.Lock()
	_, pinned := r.nsVersions[ns]
	r.mu.Unlock()
	if pinned {
		return nil
	}

	version, err := r.v.namespaceVersion(ns)
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.pinNamespaceVersionLocked(ns, version)
	return nil
}

// pinNamespaceVersionLocked records version for ns unless a version is already pinned.
func (r *rwSetWrapper) pinNamespaceVersionLocked(ns cdriver.Namespace, version cdriver.RawVersion) {
	if _, pinned := r.nsVersions[ns]; !pinned {
		r.nsVersions[ns] = version
	}
}

// IsValid validates that the whole simulation snapshot is still current: every key read
// version, and every namespace version pinned when a namespace was first touched.
//
// The namespace versions belong here because the committer validates a transaction's
// NsVersion as a read-only dependency on _meta[ns] (see the preparer in fabric-x-committer),
// so a namespace policy update invalidates a transaction exactly like a conflicting key
// read does.
//
// Reads and namespace versions are checked in one batched query rather than one call per key.
func (r *rwSetWrapper) IsValid() error {
	r.mu.Lock()
	// Copy the snapshot so the network query runs without holding the lock.
	readsCopy := make(map[cdriver.Namespace]map[string]cdriver.RawVersion, len(r.rws.Reads))
	for ns, reads := range r.rws.Reads {
		readsCopy[ns] = make(map[string]cdriver.RawVersion, len(reads))
		maps.Copy(readsCopy[ns], reads)
	}
	nsVersionsCopy := make(map[cdriver.Namespace]cdriver.RawVersion, len(r.nsVersions))
	maps.Copy(nsVersionsCopy, r.nsVersions)
	r.mu.Unlock()

	// Build the batch as key sets. The RWSet's own reads and the pinned namespaces both
	// contribute keys to _meta whenever the RWSet reads inside that namespace, so the two
	// sources have to be merged rather than concatenated.
	querySets := make(map[cdriver.Namespace]map[cdriver.PKey]struct{}, len(readsCopy)+1)
	addKey := func(ns cdriver.Namespace, key cdriver.PKey) {
		keys, ok := querySets[ns]
		if !ok {
			keys = make(map[cdriver.PKey]struct{})
			querySets[ns] = keys
		}
		keys[key] = struct{}{}
	}
	for ns, reads := range readsCopy {
		for key := range reads {
			addKey(ns, key)
		}
	}
	for ns := range nsVersionsCopy {
		addKey(metaNamespace, cdriver.PKey(ns))
	}

	// A namespace with no keys makes the query service reject the whole batch.
	query := make(map[cdriver.Namespace][]cdriver.PKey, len(querySets))
	for ns, keys := range querySets {
		if len(keys) == 0 {
			continue
		}
		query[ns] = slices.Collect(maps.Keys(keys))
	}
	if len(query) == 0 {
		return nil
	}

	states, err := r.v.queryService.GetStates(query)
	if err != nil {
		return errors.Wrapf(err, "failed to validate rwset for tx %s", string(r.txID))
	}

	for ns, reads := range readsCopy {
		for key, expectedVersion := range reads {
			current, found := states[ns][key]

			// Check version match
			if !found && expectedVersion != nil {
				return errors.Errorf("read validation failed: key %s in namespace %s was deleted", key, ns)
			}
			if found && !r.v.versionEqual(expectedVersion, current.Version) {
				return errors.Errorf("read validation failed: version mismatch for key %s in namespace %s", key, ns)
			}
		}
	}

	for ns, pinnedVersion := range nsVersionsCopy {
		current, found := states[metaNamespace][cdriver.PKey(ns)]
		// An unregistered namespace was pinned at version 0; it stays valid only while it
		// remains unregistered.
		currentVersion := MarshalVersion(0)
		if found {
			currentVersion = current.Version
		}
		if !r.v.versionEqual(pinnedVersion, currentVersion) {
			return errors.Errorf("read validation failed: namespace %s version changed since simulation", ns)
		}
	}

	return nil
}

// namespaceVersion reads the version of ns from the _meta namespace. A namespace with no
// _meta entry is reported as version 0, matching how an unregistered namespace has always
// been serialized.
func (v *Vault) namespaceVersion(ns cdriver.Namespace) (cdriver.RawVersion, error) {
	states, err := v.queryService.GetStates(map[cdriver.Namespace][]cdriver.PKey{
		metaNamespace: {cdriver.PKey(ns)},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to query %s version for namespace %s", metaNamespace, ns)
	}
	if state, ok := states[metaNamespace][cdriver.PKey(ns)]; ok {
		return state.Version, nil
	}
	return MarshalVersion(0), nil
}

// IsClosed returns whether this RWSet has been closed. Always returns false
// as RWSets in this implementation are not explicitly closed.
func (r *rwSetWrapper) IsClosed() bool {
	return false
}

// Clear removes all reads, writes, and metadata writes for the specified namespace.
// The namespace's pinned version is dropped too, so touching it again re-pins it.
//
// The ReadSet/WriteSet/MetaWriteSet Clear methods only empty the namespace's inner map,
// leaving the namespace key behind. That leftover has to go as well: Marshal ranges over
// the write and read maps and demands a pinned version for every namespace it finds, so a
// key that outlives its pin makes the next Bytes() fail.
func (r *rwSetWrapper) Clear(ns cdriver.Namespace) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rws.ReadSet.Clear(ns)
	r.rws.WriteSet.Clear(ns)
	r.rws.MetaWriteSet.Clear(ns)
	delete(r.rws.Reads, ns)
	delete(r.rws.OrderedReads, ns)
	delete(r.rws.Writes, ns)
	delete(r.rws.OrderedWrites, ns)
	delete(r.rws.MetaWrites, ns)
	delete(r.nsVersions, ns)
	r.cachedBytes = nil
	return nil
}

// AddReadAt adds a read dependency for the given namespace, key, and version to the read set.
func (r *rwSetWrapper) AddReadAt(ns cdriver.Namespace, key string, version cdriver.RawVersion) error {
	if err := r.pinNamespace(ns); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rws.ReadSet.Add(ns, key, version)
	r.cachedBytes = nil
	return nil
}

// SetState sets the value for the given namespace and key in the write set.
func (r *rwSetWrapper) SetState(namespace cdriver.Namespace, key cdriver.PKey, value cdriver.RawValue) error {
	if err := r.pinNamespace(namespace); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	err := r.rws.WriteSet.Add(namespace, key, value)
	r.cachedBytes = nil
	return err
}

// GetState retrieves the state for a given namespace and key.
// It first checks the local write set, then queries the remote storage if needed.
// The behavior can be controlled with GetStateOpt options.
//
// The write-set check, the remote query and the read-set update are three separate
// critical sections, not one atomic operation. A SetState for the same key that lands in
// between makes this method record a read even though the key is now locally written, and
// Marshal turns a key that is both read and written into a ReadWrite — a version-
// conditional write the caller never asked for, which the committer invalidates on any
// concurrent ledger update. Do not simulate one RWSet from multiple goroutines.
func (r *rwSetWrapper) GetState(namespace cdriver.Namespace, key cdriver.PKey, opts ...cdriver.GetStateOpt) (cdriver.RawValue, error) {
	// Check writes first
	r.mu.Lock()
	val, exists := r.rws.Writes[namespace][key]
	r.mu.Unlock()
	if exists {
		return val, nil
	}

	// Check if we should look in storage
	opt := cdriver.FromBoth
	if len(opts) > 0 {
		opt = opts[0]
	}

	if opt == cdriver.FromIntermediate {
		return nil, nil
	}

	// Query from storage via QueryService
	vaultValue, err := r.v.queryService.GetState(namespace, key)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get state for namespace=%s, key=%s", namespace, key)
	}
	if vaultValue == nil {
		return nil, nil
	}

	// Add to read set, pinning the namespace version alongside the key's read version.
	if err := r.pinNamespace(namespace); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rws.ReadSet.Add(namespace, key, vaultValue.Version)
	r.cachedBytes = nil

	return vaultValue.Raw, nil
}

// GetDirectState accesses the state directly from the QueryService without checking the RWSet.
// This allows accessing the query executor while having an RWSet open, avoiding nested locks.
func (r *rwSetWrapper) GetDirectState(namespace cdriver.Namespace, key cdriver.PKey) (cdriver.RawValue, error) {
	vaultValue, err := r.v.queryService.GetState(namespace, key)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get direct state for namespace=%s, key=%s", namespace, key)
	}
	if vaultValue == nil {
		return nil, nil
	}
	return vaultValue.Raw, nil
}

// DeleteState marks a key for deletion by adding a nil value to the write set.
func (r *rwSetWrapper) DeleteState(namespace cdriver.Namespace, key cdriver.PKey) error {
	if err := r.pinNamespace(namespace); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	err := r.rws.WriteSet.Add(namespace, key, nil)
	r.cachedBytes = nil
	return err
}

// GetStateMetadata retrieves metadata for a given namespace and key.
// It checks the local metadata writes first, returning nil if not found.
func (r *rwSetWrapper) GetStateMetadata(namespace cdriver.Namespace, key cdriver.PKey, opts ...cdriver.GetStateOpt) (cdriver.Metadata, error) {
	// Check in-flight meta writes first.
	r.mu.Lock()
	meta, exists := r.rws.MetaWrites[namespace][key]
	r.mu.Unlock()
	if exists {
		return meta, nil
	}

	opt := cdriver.FromBoth
	if len(opts) > 0 {
		opt = opts[0]
	}
	if opt == cdriver.FromIntermediate || r.v.mds == nil {
		return nil, nil
	}

	// Fall back to the hash-hiding field-mapping store: resolve the committed on-ledger
	// value, key by its sha256 digest, and return the stored {fieldMappingKey: mapping}
	// so getFieldMapping finds meta[fieldMappingKey] exactly as on Fabric.
	committed, err := r.v.queryService.GetState(namespace, key)
	if err != nil || committed == nil || len(committed.Raw) == 0 {
		return nil, nil
	}
	digest := sha256.Sum256(committed.Raw)
	fm, err := r.v.mds.GetFieldMapping(context.Background(), string(namespace), string(key), digest[:])
	if err != nil || len(fm) == 0 {
		return nil, nil
	}
	return cdriver.Metadata(fm), nil
}

// SetStateMetadata sets metadata for a given namespace and key in the metadata write set.
func (r *rwSetWrapper) SetStateMetadata(namespace cdriver.Namespace, key cdriver.PKey, metadata cdriver.Metadata) error {
	if err := r.pinNamespace(namespace); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	err := r.rws.MetaWriteSet.Add(namespace, key, metadata)
	r.cachedBytes = nil
	return err
}

// GetReadKeyAt returns the key of the i-th read in the specified namespace.
// Returns an error if the index is out of bounds.
func (r *rwSetWrapper) GetReadKeyAt(ns cdriver.Namespace, i int) (cdriver.PKey, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key, ok := r.rws.ReadSet.GetAt(ns, i)
	if !ok {
		return "", errors.Errorf("index %d out of bounds for namespace %s", i, ns)
	}
	return key, nil
}

// GetReadAt returns the i-th read (key, value) in the specified namespace.
// The value is loaded from the QueryService. Returns an error if the index is out of bounds
// or if the value cannot be retrieved.
func (r *rwSetWrapper) GetReadAt(ns cdriver.Namespace, i int) (cdriver.PKey, cdriver.RawValue, error) {
	key, err := r.GetReadKeyAt(ns, i)
	if err != nil {
		return "", nil, err
	}

	// Get the value from storage
	vaultValue, err := r.v.queryService.GetState(ns, key)
	if err != nil {
		return "", nil, errors.Wrapf(err, "failed to get read at index %d for namespace=%s", i, ns)
	}
	if vaultValue == nil {
		return key, nil, nil
	}

	return key, vaultValue.Raw, nil
}

// GetWriteAt returns the i-th write (key, value) in the specified namespace.
// Returns an error if the index is out of bounds.
func (r *rwSetWrapper) GetWriteAt(ns cdriver.Namespace, i int) (cdriver.PKey, cdriver.RawValue, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key, ok := r.rws.WriteSet.GetAt(ns, i)
	if !ok {
		return "", nil, errors.Errorf("index %d out of bounds for namespace %s", i, ns)
	}
	value := r.rws.Writes[ns][key]
	return key, value, nil
}

// NumReads returns the number of reads in the specified namespace.
func (r *rwSetWrapper) NumReads(ns cdriver.Namespace) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.rws.Reads[ns])
}

// NumWrites returns the number of writes in the specified namespace.
func (r *rwSetWrapper) NumWrites(ns cdriver.Namespace) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.rws.Writes[ns])
}

// Namespaces returns all namespace labels present in this RWSet (reads, writes, or metadata).
func (r *rwSetWrapper) Namespaces() []cdriver.Namespace {
	r.mu.Lock()
	defer r.mu.Unlock()

	nsMap := make(map[cdriver.Namespace]bool)
	for ns := range r.rws.Reads {
		nsMap[ns] = true
	}
	for ns := range r.rws.Writes {
		nsMap[ns] = true
	}
	for ns := range r.rws.MetaWrites {
		nsMap[ns] = true
	}

	namespaces := make([]cdriver.Namespace, 0, len(nsMap))
	for ns := range nsMap {
		namespaces = append(namespaces, ns)
	}
	return namespaces
}

// AppendRWSet deserializes and appends RWSet data from bytes to this RWSet.
// If namespaces are specified, only those namespaces will be appended.
// Namespace versions come from the appended payload rather than from the vault's
// snapshot, so the result re-serializes to the versions it arrived with.
func (r *rwSetWrapper) AppendRWSet(raw []byte, nss ...cdriver.Namespace) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	nsVersions, err := r.v.marshaller.Append(r.rws, raw, nss...)
	r.cachedBytes = nil
	if err != nil {
		return err
	}
	for ns, version := range nsVersions {
		r.pinNamespaceVersionLocked(ns, version)
	}
	return nil
}

// Bytes serializes this RWSet to bytes in FabricX protobuf format.
//
// Serialization is a pure function of RWSet state: namespace versions were resolved when
// each namespace was first touched, so Bytes() performs no network I/O and repeated calls
// return identical bytes. That matters because the endorsement digest covers NsVersion,
// and the endorsement flow serializes the same RWSet once per endorsing party.
//
// The result is memoized, but only as an optimization — the cache holds no correctness
// weight now that the inputs cannot change underneath it.
func (r *rwSetWrapper) Bytes() ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cachedBytes == nil {
		// Marshal rejects a namespace missing from nsVersions, which would mean a code path
		// mutated the RWSet without pinning a version for it.
		raw, err := r.v.marshaller.Marshal(string(r.txID), r.rws, r.nsVersions)
		if err != nil {
			return nil, err
		}
		r.cachedBytes = raw
	}

	return bytes.Clone(r.cachedBytes), nil
}

// Done is a no-op. The vault does not retain the ReadWriteSet.
func (r *rwSetWrapper) Done() {
}

// Equals compares this RWSet with another RWSet for equality.
// If namespaces are specified, only those namespaces are compared.
// Returns an error if the RWSets are not equal or if the input is not an *rwSetWrapper.
func (r *rwSetWrapper) Equals(rws any, nss ...cdriver.Namespace) error {
	other, ok := rws.(*rwSetWrapper)
	if !ok {
		return errors.Errorf("expected *rwSetWrapper, got %T", rws)
	}

	if r == other {
		r.mu.Lock()
		defer r.mu.Unlock()
	} else if uintptr(unsafe.Pointer(r)) < uintptr(unsafe.Pointer(other)) {
		r.mu.Lock()
		defer r.mu.Unlock()
		other.mu.Lock()
		defer other.mu.Unlock()
	} else {
		other.mu.Lock()
		defer other.mu.Unlock()
		r.mu.Lock()
		defer r.mu.Unlock()
	}

	if err := r.rws.Reads.Equals(other.rws.Reads, nss...); err != nil {
		return err
	}
	if err := r.rws.Writes.Equals(other.rws.Writes, nss...); err != nil {
		return err
	}
	if err := r.rws.MetaWrites.Equals(other.rws.MetaWrites, nss...); err != nil {
		return err
	}
	return nil
}

// NewRWSet creates a new empty RWSet for the given transaction ID.
// The returned wrapper owns the ReadWriteSet; the vault does not retain a reference to it.
func (v *Vault) NewRWSet(ctx context.Context, txID cdriver.TxID) (cdriver.RWSet, error) {
	rws := vault.EmptyRWSet()

	qe, err := v.NewQueryExecutor(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create query executor")
	}

	return &rwSetWrapper{
		txID:       txID,
		rws:        &rws,
		qe:         qe,
		v:          v,
		nsVersions: make(map[cdriver.Namespace]cdriver.RawVersion),
	}, nil
}

// NewRWSetFromBytes creates a new RWSet by deserializing it from bytes.
// The returned wrapper owns the ReadWriteSet; the vault does not retain a reference to it.
// Namespace versions are taken from the serialized transaction, so re-serializing the
// result reproduces the bytes it was built from — which is what lets an endorser sign
// over the same digest the proposer produced.
func (v *Vault) NewRWSetFromBytes(ctx context.Context, txID cdriver.TxID, rwset []byte) (cdriver.RWSet, error) {
	rws, nsVersions, err := v.marshaller.RWSetFromBytes(rwset)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal rwset")
	}

	qe, err := v.NewQueryExecutor(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create query executor")
	}

	return &rwSetWrapper{
		txID:       txID,
		rws:        rws,
		qe:         qe,
		v:          v,
		nsVersions: nsVersions,
	}, nil
}

// SetDiscarded is not supported: the fabricx wiring has no local commit pipeline
// (see platform/fabricx/core/channel/provider.go). Reaching this method means the
// vault was wired into a generic committer, which is a programming error.
func (v *Vault) SetDiscarded(context.Context, cdriver.TxID, string) error {
	panic("fabricx vault: SetDiscarded called; fabricx has no local commit pipeline")
}

// Status returns the validation status of a single transaction. It delegates to Statuses so that
// the two methods answer the same question the same way: a txID the committer does not yet know is
// reported as Unknown ("not final yet") rather than as an error, which matters for callers polling
// an in-flight transaction through the public vault facade (platform/fabric/vault.go).
func (v *Vault) Status(ctx context.Context, txID cdriver.TxID) (fdriver.ValidationCode, string, error) {
	statuses, err := v.Statuses(ctx, txID)
	if err != nil {
		return fdriver.Unknown, "", errors.Wrapf(err, "failed to get transaction status for txID=%s", txID)
	}
	return statuses[0].ValidationCode, statuses[0].Message, nil
}

// Statuses returns the validation statuses for multiple transactions, resolving them from the
// remote QueryService in a single batched query. A txID that the committer does not know
// is omitted from the batched result and is reported here as Unknown
// ("not final yet"). The result preserves the order of the input txIDs.
func (v *Vault) Statuses(ctx context.Context, txIDs ...cdriver.TxID) ([]cdriver.TxValidationStatus[fdriver.ValidationCode], error) {
	ids := make([]string, len(txIDs))
	for i, txID := range txIDs {
		ids[i] = string(txID)
	}

	codes, err := v.queryService.GetTransactionStatuses(ids)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get transaction statuses")
	}

	statuses := make([]cdriver.TxValidationStatus[fdriver.ValidationCode], len(txIDs))
	for i, txID := range txIDs {
		code := fdriver.Unknown // omitted from the batched result => not final yet
		if statusCode, ok := codes[string(txID)]; ok {
			code = v.mapStatusToValidationCode(statusCode)
		}
		statuses[i] = cdriver.TxValidationStatus[fdriver.ValidationCode]{
			TxID:           txID,
			ValidationCode: code,
		}
	}
	return statuses, nil
}

// DiscardTx is not supported: the fabricx wiring has no local commit pipeline
// (see platform/fabricx/core/channel/provider.go). Reaching this method means the
// vault was wired into a generic committer, which is a programming error.
func (v *Vault) DiscardTx(context.Context, cdriver.TxID, string) error {
	panic("fabricx vault: DiscardTx called; fabricx has no local commit pipeline")
}

// CommitTX is not supported: the fabricx wiring has no local commit pipeline
// (see platform/fabricx/core/channel/provider.go). Reaching this method means the
// vault was wired into a generic committer, which is a programming error.
func (v *Vault) CommitTX(context.Context, cdriver.TxID, cdriver.BlockNum, cdriver.TxNum) error {
	panic("fabricx vault: CommitTX called; fabricx has no local commit pipeline")
}

// InspectRWSet creates an ephemeral RWSet from bytes for inspection purposes.
// If namespaces are specified, only those namespaces will be included.
// The RWSet is not stored locally (ephemeral).
func (v *Vault) InspectRWSet(ctx context.Context, rwset []byte, namespaces ...cdriver.Namespace) (cdriver.RWSet, error) {
	rws, nsVersions, err := v.marshaller.RWSetFromBytes(rwset, namespaces...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal rwset for inspection")
	}

	qe, err := v.NewQueryExecutor(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create query executor")
	}

	// Return ephemeral RWSet (not stored in local map)
	return &rwSetWrapper{
		txID:       "", // Empty txID for ephemeral rwset
		rws:        rws,
		qe:         qe,
		v:          v,
		nsVersions: nsVersions,
	}, nil
}

// RWSExists is not supported: the fabricx wiring has no local commit pipeline
// (see platform/fabricx/core/channel/provider.go). Reaching this method means the
// vault was wired into a generic committer, which is a programming error.
func (v *Vault) RWSExists(context.Context, cdriver.TxID) bool {
	panic("fabricx vault: RWSExists called; fabricx has no local commit pipeline")
}

// Match is not supported: the fabricx wiring has no local commit pipeline
// (see platform/fabricx/core/channel/provider.go). Reaching this method means the
// vault was wired into a generic committer, which is a programming error.
func (v *Vault) Match(context.Context, cdriver.TxID, []byte) error {
	panic("fabricx vault: Match called; fabricx has no local commit pipeline")
}

// Close is a no-op. The vault holds no per-transaction state to release.
func (v *Vault) Close() error {
	return nil
}

// versionEqual compares two version byte slices for equality.
func (v *Vault) versionEqual(v1, v2 cdriver.RawVersion) bool {
	if len(v1) != len(v2) {
		return false
	}
	for i := range v1 {
		if v1[i] != v2[i] {
			return false
		}
	}
	return true
}

// mapStatusToValidationCode converts a committerpb.Status code to a fdriver.ValidationCode.
// Maps COMMITTED to Valid, STATUS_UNSPECIFIED to Unknown, and all others to Invalid.
func (v *Vault) mapStatusToValidationCode(statusCode int32) fdriver.ValidationCode {
	// Map committerpb.Status to fdriver.ValidationCode
	switch committerpb.Status(statusCode) {
	case committerpb.Status_COMMITTED:
		return fdriver.Valid
	case committerpb.Status_STATUS_UNSPECIFIED:
		return fdriver.Unknown
	default:
		return fdriver.Invalid
	}
}
