/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"context"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault"
	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice"
	"github.com/hyperledger/fabric-x-common/api/committerpb"
)

// txStatusInfo holds local transaction status information including validation code,
// error message, and block/transaction index for committed transactions.
type txStatusInfo struct {
	code    fdriver.ValidationCode // Validation code (Valid, Invalid, Unknown, etc.)
	message string                 // Error or status message
	block   cdriver.BlockNum       // Block number where transaction was committed
	index   cdriver.TxNum          // Transaction index within the block
}

// Vault is a vault implementation that uses QueryService for remote state queries
// and manages read-write sets (RWSets) and transaction statuses locally.
//
// It provides a hybrid architecture where:
//   - Read operations are delegated to a remote QueryService
//   - RWSets are managed in-memory for performance
//   - Transaction statuses are cached locally with remote fallback
//   - All operations are thread-safe using RWMutex
//
// Vault implements the fdriver.Vault interface for FabricX.
type Vault struct {
	queryService queryservice.QueryService            // Remote query service for state queries
	marshaller   *Marshaller                          // Marshaller for RWSet serialization
	rwsets       map[cdriver.TxID]*vault.ReadWriteSet // Local RWSet storage
	txStatuses   map[cdriver.TxID]*txStatusInfo       // Local transaction status cache
	mu           sync.RWMutex                         // Mutex for thread-safe access
}

// NewVault creates a new Vault instance with the given QueryService.
// The vault will use the query service for all remote state queries and transaction status lookups.
//
// Parameters:
//   - qs: QueryService instance for remote queries
//
// Returns:
//   - *Vault: A new vault instance ready for use
func NewVault(qs queryservice.QueryService) *Vault {
	return &Vault{
		queryService: qs,
		marshaller:   NewMarshaller(),
		rwsets:       make(map[cdriver.TxID]*vault.ReadWriteSet),
		txStatuses:   make(map[cdriver.TxID]*txStatusInfo),
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
func (qe *queryExecutor) GetStateRange(ctx context.Context, namespace cdriver.Namespace, startKey cdriver.PKey, endKey cdriver.PKey) (cdriver.VersionedResultsIterator, error) {
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
type rwSetWrapper struct {
	txID cdriver.TxID          // Transaction ID
	rws  *vault.ReadWriteSet   // Underlying read-write set
	qe   cdriver.QueryExecutor // Query executor for state queries
	v    *Vault                // Parent vault for accessing query service
}

// IsValid validates that all reads in the RWSet are still valid by checking
// that the versions in the ledger match the versions in the read set.
// Returns an error if any read is invalid (version mismatch or key deleted).
func (r *rwSetWrapper) IsValid() error {
	// Validate that all reads are still valid
	for ns, reads := range r.rws.Reads {
		for key, expectedVersion := range reads {
			vaultValue, err := r.v.queryService.GetState(ns, key)
			if err != nil {
				return errors.Wrapf(err, "failed to validate read for namespace=%s, key=%s", ns, key)
			}

			// Check version match
			if vaultValue == nil && expectedVersion != nil {
				return errors.Errorf("read validation failed: key %s in namespace %s was deleted", key, ns)
			}
			if vaultValue != nil && !r.v.versionEqual(expectedVersion, vaultValue.Version) {
				return errors.Errorf("read validation failed: version mismatch for key %s in namespace %s", key, ns)
			}
		}
	}
	return nil
}

// IsClosed returns whether this RWSet has been closed. Always returns false
// as RWSets in this implementation are not explicitly closed.
func (r *rwSetWrapper) IsClosed() bool {
	return false
}

// Clear removes all reads, writes, and metadata writes for the specified namespace.
func (r *rwSetWrapper) Clear(ns cdriver.Namespace) error {
	r.rws.ReadSet.Clear(ns)
	r.rws.WriteSet.Clear(ns)
	r.rws.MetaWriteSet.Clear(ns)
	return nil
}

// AddReadAt adds a read dependency for the given namespace, key, and version to the read set.
func (r *rwSetWrapper) AddReadAt(ns cdriver.Namespace, key string, version cdriver.RawVersion) error {
	r.rws.ReadSet.Add(ns, key, version)
	return nil
}

// SetState sets the value for the given namespace and key in the write set.
func (r *rwSetWrapper) SetState(namespace cdriver.Namespace, key cdriver.PKey, value cdriver.RawValue) error {
	return r.rws.WriteSet.Add(namespace, key, value)
}

// GetState retrieves the state for a given namespace and key.
// It first checks the local write set, then queries the remote storage if needed.
// The behavior can be controlled with GetStateOpt options.
func (r *rwSetWrapper) GetState(namespace cdriver.Namespace, key cdriver.PKey, opts ...cdriver.GetStateOpt) (cdriver.RawValue, error) {
	// Check writes first
	if val, exists := r.rws.Writes[namespace][key]; exists {
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

	// Add to read set
	r.rws.ReadSet.Add(namespace, key, vaultValue.Version)

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
	return r.rws.WriteSet.Add(namespace, key, nil)
}

// GetStateMetadata retrieves metadata for a given namespace and key.
// It checks the local metadata writes first, returning nil if not found.
func (r *rwSetWrapper) GetStateMetadata(namespace cdriver.Namespace, key cdriver.PKey, opts ...cdriver.GetStateOpt) (cdriver.Metadata, error) {
	// Check meta writes first
	if meta, exists := r.rws.MetaWrites[namespace][key]; exists {
		return meta, nil
	}
	return nil, nil
}

// SetStateMetadata sets metadata for a given namespace and key in the metadata write set.
func (r *rwSetWrapper) SetStateMetadata(namespace cdriver.Namespace, key cdriver.PKey, metadata cdriver.Metadata) error {
	return r.rws.MetaWriteSet.Add(namespace, key, metadata)
}

// GetReadKeyAt returns the key of the i-th read in the specified namespace.
// Returns an error if the index is out of bounds.
func (r *rwSetWrapper) GetReadKeyAt(ns cdriver.Namespace, i int) (cdriver.PKey, error) {
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
	key, ok := r.rws.WriteSet.GetAt(ns, i)
	if !ok {
		return "", nil, errors.Errorf("index %d out of bounds for namespace %s", i, ns)
	}
	value := r.rws.Writes[ns][key]
	return key, value, nil
}

// NumReads returns the number of reads in the specified namespace.
func (r *rwSetWrapper) NumReads(ns cdriver.Namespace) int {
	return len(r.rws.Reads[ns])
}

// NumWrites returns the number of writes in the specified namespace.
func (r *rwSetWrapper) NumWrites(ns cdriver.Namespace) int {
	return len(r.rws.Writes[ns])
}

// Namespaces returns all namespace labels present in this RWSet (reads, writes, or metadata).
func (r *rwSetWrapper) Namespaces() []cdriver.Namespace {
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
func (r *rwSetWrapper) AppendRWSet(raw []byte, nss ...cdriver.Namespace) error {
	return r.v.marshaller.Append(r.rws, raw, nss...)
}

// Bytes serializes this RWSet to bytes in FabricX protobuf format.
// It automatically fetches namespace versions from the _meta namespace via QueryService.
func (r *rwSetWrapper) Bytes() ([]byte, error) {
	// Get namespace versions from the query service
	namespaces := r.Namespaces()
	nsInfo := make(map[cdriver.Namespace]cdriver.RawVersion)

	for _, ns := range namespaces {
		// Try to get namespace version from _meta namespace
		vaultValue, err := r.v.queryService.GetState("_meta", ns)
		if err != nil {
			// If error, use version 0
			nsInfo[ns] = MarshalVersion(0)
		} else if vaultValue == nil {
			// If not found, use version 0
			nsInfo[ns] = MarshalVersion(0)
		} else {
			// Use the version from _meta
			nsInfo[ns] = vaultValue.Version
		}
	}

	// Set nsInfo in marshaller before marshalling
	r.v.marshaller.NsInfo = nsInfo

	return r.v.marshaller.Marshal(string(r.txID), r.rws)
}

// Done performs cleanup for this RWSet. Currently a no-op as no cleanup is needed.
func (r *rwSetWrapper) Done() {
	// No cleanup needed
}

// Equals compares this RWSet with another RWSet for equality.
// If namespaces are specified, only those namespaces are compared.
// Returns an error if the RWSets are not equal or if the input is not an *rwSetWrapper.
func (r *rwSetWrapper) Equals(rws interface{}, nss ...cdriver.Namespace) error {
	other, ok := rws.(*rwSetWrapper)
	if !ok {
		return errors.Errorf("expected *rwSetWrapper, got %T", rws)
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
// The RWSet is stored locally and can be used to track reads and writes.
func (v *Vault) NewRWSet(ctx context.Context, txID cdriver.TxID) (cdriver.RWSet, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	rws := vault.EmptyRWSet()
	v.rwsets[txID] = &rws

	qe, err := v.NewQueryExecutor(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create query executor")
	}

	return &rwSetWrapper{
		txID: txID,
		rws:  &rws,
		qe:   qe,
		v:    v,
	}, nil
}

// NewRWSetFromBytes creates a new RWSet by deserializing it from bytes.
// The RWSet is stored locally with the given transaction ID.
func (v *Vault) NewRWSetFromBytes(ctx context.Context, txID cdriver.TxID, rwset []byte) (cdriver.RWSet, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	rws, err := v.marshaller.RWSetFromBytes(rwset)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal rwset")
	}

	v.rwsets[txID] = rws

	qe, err := v.NewQueryExecutor(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create query executor")
	}

	return &rwSetWrapper{
		txID: txID,
		rws:  rws,
		qe:   qe,
		v:    v,
	}, nil
}

// SetDiscarded marks a transaction as discarded (invalid) with the given error message.
// The status is stored locally.
func (v *Vault) SetDiscarded(ctx context.Context, txID cdriver.TxID, message string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.txStatuses[txID] = &txStatusInfo{
		code:    fdriver.Invalid,
		message: message,
	}
	return nil
}

// Status returns the validation status of a transaction.
// It first checks the local cache, then queries the remote QueryService if not found locally.
// The result from the remote service is cached for future queries.
func (v *Vault) Status(ctx context.Context, txID cdriver.TxID) (fdriver.ValidationCode, string, error) {
	v.mu.RLock()
	// Check local cache first
	if status, exists := v.txStatuses[txID]; exists {
		v.mu.RUnlock()
		return status.code, status.message, nil
	}
	v.mu.RUnlock()

	// Query remote service
	statusCode, err := v.queryService.GetTransactionStatus(string(txID))
	if err != nil {
		return fdriver.Unknown, "", errors.Wrapf(err, "failed to get transaction status for txID=%s", txID)
	}

	// Map status code to ValidationCode
	validationCode := v.mapStatusToValidationCode(statusCode)

	// Cache the result
	v.mu.Lock()
	v.txStatuses[txID] = &txStatusInfo{
		code:    validationCode,
		message: "",
	}
	v.mu.Unlock()

	return validationCode, "", nil
}

// Statuses returns the validation statuses for multiple transactions.
// Each transaction status is retrieved using the Status method.
func (v *Vault) Statuses(ctx context.Context, txIDs ...cdriver.TxID) ([]cdriver.TxValidationStatus[fdriver.ValidationCode], error) {
	statuses := make([]cdriver.TxValidationStatus[fdriver.ValidationCode], len(txIDs))

	for i, txID := range txIDs {
		code, message, err := v.Status(ctx, txID)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get status for txID=%s", txID)
		}
		statuses[i] = cdriver.TxValidationStatus[fdriver.ValidationCode]{
			TxID:           txID,
			ValidationCode: code,
			Message:        message,
		}
	}

	return statuses, nil
}

// DiscardTx marks a transaction as discarded. This is an alias for SetDiscarded.
func (v *Vault) DiscardTx(ctx context.Context, txID cdriver.TxID, message string) error {
	return v.SetDiscarded(ctx, txID, message)
}

// CommitTX marks a transaction as committed (valid) and records its block number and index.
// The status is stored locally.
func (v *Vault) CommitTX(ctx context.Context, txID cdriver.TxID, block cdriver.BlockNum, index cdriver.TxNum) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.txStatuses[txID] = &txStatusInfo{
		code:    fdriver.Valid,
		message: "",
		block:   block,
		index:   index,
	}
	return nil
}

// InspectRWSet creates an ephemeral RWSet from bytes for inspection purposes.
// If namespaces are specified, only those namespaces will be included.
// The RWSet is not stored locally (ephemeral).
func (v *Vault) InspectRWSet(ctx context.Context, rwset []byte, namespaces ...cdriver.Namespace) (cdriver.RWSet, error) {
	rws, err := v.marshaller.RWSetFromBytes(rwset, namespaces...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal rwset for inspection")
	}

	qe, err := v.NewQueryExecutor(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create query executor")
	}

	// Return ephemeral RWSet (not stored in local map)
	return &rwSetWrapper{
		txID: "", // Empty txID for ephemeral rwset
		rws:  rws,
		qe:   qe,
		v:    v,
	}, nil
}

// RWSExists checks whether an RWSet exists locally for the given transaction ID.
func (v *Vault) RWSExists(ctx context.Context, id cdriver.TxID) bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	_, exists := v.rwsets[id]
	return exists
}

// Match compares a stored RWSet with provided bytes to verify they match.
// Returns an error if the RWSet doesn't exist, or if the bytes don't match.
func (v *Vault) Match(ctx context.Context, id cdriver.TxID, results []byte) error {
	v.mu.RLock()
	rws, exists := v.rwsets[id]
	v.mu.RUnlock()

	if !exists {
		return errors.Errorf("rwset not found for txID=%s", id)
	}

	// Marshal the stored RWSet
	storedBytes, err := v.marshaller.Marshal(string(id), rws)
	if err != nil {
		return errors.Wrap(err, "failed to marshal stored rwset")
	}

	// Compare bytes
	if len(storedBytes) != len(results) {
		return errors.Errorf("rwset mismatch: length differs (stored=%d, provided=%d)", len(storedBytes), len(results))
	}

	for i := range storedBytes {
		if storedBytes[i] != results[i] {
			return errors.Errorf("rwset mismatch at byte %d", i)
		}
	}

	return nil
}

// Close clears all local storage (RWSets and transaction statuses).
// After closing, the vault can still be used but all cached data is lost.
func (v *Vault) Close() error {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Clear local storage
	v.rwsets = make(map[cdriver.TxID]*vault.ReadWriteSet)
	v.txStatuses = make(map[cdriver.TxID]*txStatusInfo)

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
