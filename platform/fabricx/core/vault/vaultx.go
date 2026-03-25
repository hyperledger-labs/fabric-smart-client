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

// txStatusInfo holds local transaction status information
type txStatusInfo struct {
	code    fdriver.ValidationCode
	message string
	block   cdriver.BlockNum
	index   cdriver.TxNum
}

// VaultX is a vault implementation that uses QueryService for remote queries
// and manages RWSets locally
type VaultX struct {
	queryService queryservice.QueryService
	marshaller   *Marshaller
	rwsets       map[cdriver.TxID]*vault.ReadWriteSet
	txStatuses   map[cdriver.TxID]*txStatusInfo
	mu           sync.RWMutex
}

// NewVaultX creates a new VaultX instance with the given QueryService
func NewVaultX(qs queryservice.QueryService) *VaultX {
	return &VaultX{
		queryService: qs,
		marshaller:   NewMarshaller(),
		rwsets:       make(map[cdriver.TxID]*vault.ReadWriteSet),
		txStatuses:   make(map[cdriver.TxID]*txStatusInfo),
	}
}

// queryExecutor wraps the QueryService to implement cdriver.QueryExecutor
type queryExecutor struct {
	qs  queryservice.QueryService
	ctx context.Context
}

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

func (qe *queryExecutor) GetStateRange(ctx context.Context, namespace cdriver.Namespace, startKey cdriver.PKey, endKey cdriver.PKey) (cdriver.VersionedResultsIterator, error) {
	// QueryService doesn't support range queries
	return nil, errors.New("GetStateRange not supported by VaultX QueryService")
}

func (qe *queryExecutor) Done() error {
	// No cleanup needed for query executor
	return nil
}

func (v *VaultX) NewQueryExecutor(ctx context.Context) (cdriver.QueryExecutor, error) {
	return &queryExecutor{
		qs:  v.queryService,
		ctx: ctx,
	}, nil
}

// rwSetWrapper wraps a ReadWriteSet to implement cdriver.RWSet
type rwSetWrapper struct {
	txID cdriver.TxID
	rws  *vault.ReadWriteSet
	qe   cdriver.QueryExecutor
	v    *VaultX
}

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

func (r *rwSetWrapper) IsClosed() bool {
	return false
}

func (r *rwSetWrapper) Clear(ns cdriver.Namespace) error {
	r.rws.ReadSet.Clear(ns)
	r.rws.WriteSet.Clear(ns)
	r.rws.MetaWriteSet.Clear(ns)
	return nil
}

func (r *rwSetWrapper) AddReadAt(ns cdriver.Namespace, key string, version cdriver.RawVersion) error {
	r.rws.ReadSet.Add(ns, key, version)
	return nil
}

func (r *rwSetWrapper) SetState(namespace cdriver.Namespace, key cdriver.PKey, value cdriver.RawValue) error {
	return r.rws.WriteSet.Add(namespace, key, value)
}

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

func (r *rwSetWrapper) DeleteState(namespace cdriver.Namespace, key cdriver.PKey) error {
	return r.rws.WriteSet.Add(namespace, key, nil)
}

func (r *rwSetWrapper) GetStateMetadata(namespace cdriver.Namespace, key cdriver.PKey, opts ...cdriver.GetStateOpt) (cdriver.Metadata, error) {
	// Check meta writes first
	if meta, exists := r.rws.MetaWrites[namespace][key]; exists {
		return meta, nil
	}
	return nil, nil
}

func (r *rwSetWrapper) SetStateMetadata(namespace cdriver.Namespace, key cdriver.PKey, metadata cdriver.Metadata) error {
	return r.rws.MetaWriteSet.Add(namespace, key, metadata)
}

func (r *rwSetWrapper) GetReadKeyAt(ns cdriver.Namespace, i int) (cdriver.PKey, error) {
	key, ok := r.rws.ReadSet.GetAt(ns, i)
	if !ok {
		return "", errors.Errorf("index %d out of bounds for namespace %s", i, ns)
	}
	return key, nil
}

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

func (r *rwSetWrapper) GetWriteAt(ns cdriver.Namespace, i int) (cdriver.PKey, cdriver.RawValue, error) {
	key, ok := r.rws.WriteSet.GetAt(ns, i)
	if !ok {
		return "", nil, errors.Errorf("index %d out of bounds for namespace %s", i, ns)
	}
	value := r.rws.Writes[ns][key]
	return key, value, nil
}

func (r *rwSetWrapper) NumReads(ns cdriver.Namespace) int {
	return len(r.rws.Reads[ns])
}

func (r *rwSetWrapper) NumWrites(ns cdriver.Namespace) int {
	return len(r.rws.Writes[ns])
}

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

func (r *rwSetWrapper) AppendRWSet(raw []byte, nss ...cdriver.Namespace) error {
	return r.v.marshaller.Append(r.rws, raw, nss...)
}

func (r *rwSetWrapper) Bytes() ([]byte, error) {
	// Get namespace versions from the query service
	namespaces := r.Namespaces()
	nsInfo := make(map[cdriver.Namespace]cdriver.RawVersion)

	for _, ns := range namespaces {
		// Try to get namespace version from _meta namespace
		vaultValue, err := r.v.queryService.GetState("_meta", ns)
		if err != nil {
			// If error, use version 0
			nsInfo[ns] = Marshal(0)
		} else if vaultValue == nil {
			// If not found, use version 0
			nsInfo[ns] = Marshal(0)
		} else {
			// Use the version from _meta
			nsInfo[ns] = vaultValue.Version
		}
	}

	// Set nsInfo in marshaller before marshalling
	r.v.marshaller.NsInfo = nsInfo

	return r.v.marshaller.Marshal(string(r.txID), r.rws)
}

func (r *rwSetWrapper) Done() {
	// No cleanup needed
}

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

func (v *VaultX) NewRWSet(ctx context.Context, txID cdriver.TxID) (cdriver.RWSet, error) {
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

func (v *VaultX) NewRWSetFromBytes(ctx context.Context, txID cdriver.TxID, rwset []byte) (cdriver.RWSet, error) {
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

func (v *VaultX) SetDiscarded(ctx context.Context, txID cdriver.TxID, message string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.txStatuses[txID] = &txStatusInfo{
		code:    fdriver.Invalid,
		message: message,
	}
	return nil
}

func (v *VaultX) Status(ctx context.Context, txID cdriver.TxID) (fdriver.ValidationCode, string, error) {
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

func (v *VaultX) Statuses(ctx context.Context, txIDs ...cdriver.TxID) ([]cdriver.TxValidationStatus[fdriver.ValidationCode], error) {
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

func (v *VaultX) DiscardTx(ctx context.Context, txID cdriver.TxID, message string) error {
	return v.SetDiscarded(ctx, txID, message)
}

func (v *VaultX) CommitTX(ctx context.Context, txID cdriver.TxID, block cdriver.BlockNum, index cdriver.TxNum) error {
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

func (v *VaultX) InspectRWSet(ctx context.Context, rwset []byte, namespaces ...cdriver.Namespace) (cdriver.RWSet, error) {
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

func (v *VaultX) RWSExists(ctx context.Context, id cdriver.TxID) bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	_, exists := v.rwsets[id]
	return exists
}

func (v *VaultX) Match(ctx context.Context, id cdriver.TxID, results []byte) error {
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

func (v *VaultX) Close() error {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Clear local storage
	v.rwsets = make(map[cdriver.TxID]*vault.ReadWriteSet)
	v.txStatuses = make(map[cdriver.TxID]*txStatusInfo)

	return nil
}

// Helper methods

func (v *VaultX) versionEqual(v1, v2 cdriver.RawVersion) bool {
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

func (v *VaultX) mapStatusToValidationCode(statusCode int32) fdriver.ValidationCode {
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

// Made with Bob
