/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/gob"
	"sync"
	"sync/atomic"

	"google.golang.org/protobuf/proto"

	"github.com/hyperledger-labs/fabric-smart-client/internal/storage/sqlbuild"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/protos"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/pagination"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
)

var logger = logging.MustGetLogger()

type VaultTables struct {
	StateTable  string
	StatusTable string
}

// KeyEncoder converts a state key into the query parameter bound to the pkey
// column of the state table. Point lookups and range scans compare the bound
// parameter with the stored column, so the encoding must be injective and the
// column must order encoded keys bytewise, as Fabric orders keys. Composite keys
// contain U+0000, so the column type must accept it.
type KeyEncoder func(driver.PKey) any

// NewVaultStore creates a VaultStore. All read and write queries use $N
// placeholders, which both SQLite and Postgres accept. encodeKey converts each
// state key before it is bound to a query; stored keys are scanned back into
// strings unchanged. timeout bounds each statement; a backend's Store applies it
// to its whole write transaction.
func NewVaultStore(writeDB common.WriteDB, readDB *sql.DB, tables VaultTables, errorWrapper dbdriver.SQLErrorWrapper, encodeKey KeyEncoder, il common.IsolationLevelMapper, timeout common.QueryTimeout) *VaultStore {
	return &VaultStore{
		vaultReader: &vaultReader{
			readDB:       readDB,
			encodeKey:    encodeKey,
			tables:       tables,
			errorWrapper: errorWrapper,
			timeout:      timeout,
		},
		tables:       tables,
		errorWrapper: errorWrapper,
		readDB:       readDB,
		writeDB:      writeDB,
		il:           il,
		Timeout:      timeout,
	}
}

// VaultStore is the SQL vault shared by the SQLite and Postgres drivers. It stores each key's
// metadata gob-encoded until the vault's contract step 2 (see [VaultStore.MetadataToProtobuf])
// has run, and as protobuf [protos.VaultMetadata] afterwards; readers accept both forms.
type VaultStore struct {
	*vaultReader
	tables       VaultTables
	errorWrapper dbdriver.SQLErrorWrapper
	readDB       *sql.DB
	writeDB      common.WriteDB
	GlobalLock   sync.RWMutex
	il           common.IsolationLevelMapper
	// Timeout bounds each statement of the store, and the write transaction of a
	// backend's Store as a whole.
	Timeout common.QueryTimeout
	// readableBy is the vault's readable_by as of its last open; see SetReadableBy.
	readableBy atomic.Int32
}

// SetReadableBy implements migrate.ReadableByObserver: from readable_by 2 on, the vault
// writes metadata as protobuf.
func (db *VaultStore) SetReadableBy(v int) { db.readableBy.Store(int32(v)) } //nolint:gosec // a step number

func (db *VaultStore) NewTxLockVaultReader(ctx context.Context, txID driver.TxID, isolationLevel driver.IsolationLevel) (driver.LockedVaultReader, error) {
	logger.DebugfContext(ctx, "acquire tx id lock for [%s]", txID)

	// Ignore conflicts in case replicas create the same entry when receiving the envelope
	query, params := sqlbuild.New().
		WriteString("INSERT INTO " + db.tables.StatusTable + " (tx_id,code) VALUES ").
		WriteTuples([]sqlbuild.Tuple{{txID, driver.Busy}}).
		WriteString(" ON CONFLICT DO NOTHING").
		Build()
	logger.Debug(query, params)

	if err := db.exec(ctx, query, params...); err != nil {
		return nil, db.errorWrapper.WrapError(err)
	}

	return newTxVaultReader(func() (*vaultReader, releaseFunc, error) {
		return db.buildTxLockVaultReader(ctx, isolationLevel)
	}), nil
}

func (db *VaultStore) buildTxLockVaultReader(ctx context.Context, isolationLevel driver.IsolationLevel) (*vaultReader, releaseFunc, error) {
	il, err := db.il.Map(isolationLevel)
	if err != nil {
		return nil, nil, err
	}
	tx, err := db.readDB.BeginTx(ctx, &sql.TxOptions{
		Isolation: il,
		ReadOnly:  true,
	})
	if err != nil {
		return nil, nil, db.errorWrapper.WrapError(err)
	}
	// The transaction lasts the whole endorsement, so it has no deadline; each
	// statement in it has the query timeout.
	return &vaultReader{
		readDB:       tx,
		encodeKey:    db.encodeKey,
		tables:       db.tables,
		errorWrapper: db.errorWrapper,
		timeout:      db.Timeout,
	}, tx.Commit, nil
}

func (db *VaultStore) NewGlobalLockVaultReader(ctx context.Context) (driver.LockedVaultReader, error) {
	logger.DebugfContext(ctx, "start acquire global lock")
	defer logger.DebugfContext(ctx, "end acquire global lock")
	return newTxVaultReader(db.buildGlobalLockVaultReader), nil
}

func (db *VaultStore) buildGlobalLockVaultReader() (*vaultReader, releaseFunc, error) {
	db.GlobalLock.Lock()
	release := func() error {
		db.GlobalLock.Unlock()
		return nil
	}
	return &vaultReader{
		readDB:       db.readDB,
		encodeKey:    db.encodeKey,
		tables:       db.tables,
		errorWrapper: db.errorWrapper,
		timeout:      db.Timeout,
	}, release, nil
}

// SetStatuses sets code and message for every transaction in txIDs. A Busy code does not
// replace a Valid status, so a committed transaction stays committed.
func (db *VaultStore) SetStatuses(ctx context.Context, code driver.TxStatusCode, message string, txIDs ...driver.TxID) error {
	db.GlobalLock.RLock()
	defer db.GlobalLock.RUnlock()

	if len(txIDs) == 0 {
		return errors.Wrap(sqlbuild.ErrNoValues, "failed to build query")
	}
	rows := make([]sqlbuild.Tuple, len(txIDs))
	for i, txID := range txIDs {
		rows[i] = sqlbuild.Tuple{txID, code, message}
	}

	b := sqlbuild.New().
		WriteString("INSERT INTO " + db.tables.StatusTable + " (tx_id,code,message) VALUES ").
		WriteTuples(rows).
		WriteString(" ON CONFLICT (tx_id) DO UPDATE SET code = EXCLUDED.code, message = EXCLUDED.message")
	if code == driver.Busy {
		db.keepValid(b)
	}
	query, params := b.Build()

	logger.Debug(query, params)

	if err := db.exec(ctx, query, params...); err != nil {
		return errors.Wrapf(db.errorWrapper.WrapError(err), "failed updating statuses for %d txids", len(txIDs))
	}
	return nil
}

// exec runs one statement on the write pool under the store's query timeout.
func (db *VaultStore) exec(ctx context.Context, query string, params ...any) error {
	ctx, cancel := db.Timeout.Context(ctx)
	defer cancel()
	_, err := db.writeDB.ExecContext(ctx, query, params...)
	return err
}

// SetStatusesBusy builds an INSERT ... ON CONFLICT DO UPDATE statement that marks all
// transactions in txIDs as Busy in the status table, except those already Valid. It
// returns the SQL query string and the corresponding argument slice ready for
// execution, or ErrNoValues if txIDs is empty.
func (db *VaultStore) SetStatusesBusy(txIDs []driver.TxID) (string, []any, error) {
	if len(txIDs) == 0 {
		return "", nil, sqlbuild.ErrNoValues
	}
	rows := make([]sqlbuild.Tuple, len(txIDs))
	for i, txID := range txIDs {
		rows[i] = sqlbuild.Tuple{txID, driver.Busy}
	}

	b := sqlbuild.New().
		WriteString("INSERT INTO " + db.tables.StatusTable + " (tx_id,code) VALUES ").
		WriteTuples(rows).
		WriteString(" ON CONFLICT (tx_id) DO UPDATE SET code = EXCLUDED.code")
	db.keepValid(b)
	query, params := b.Build()
	return query, params, nil
}

// keepValid restricts the ON CONFLICT DO UPDATE clause written to b to rows that are
// not Valid.
func (db *VaultStore) keepValid(b *sqlbuild.Builder) {
	b.WriteString(" WHERE " + db.tables.StatusTable + ".code <> ").WriteParam(driver.Valid)
}

func (db *VaultStore) UpsertStates(writes driver.Writes, metaWrites driver.MetaWrites) (string, []any, error) {
	states, err := db.convertStateRows(writes, metaWrites)
	if err != nil {
		return "", nil, err
	}
	if len(states) == 0 {
		return "", nil, sqlbuild.ErrNoValues
	}

	query, params := sqlbuild.New().
		WriteString("INSERT INTO " + db.tables.StateTable + " (ns,pkey,val,kversion,metadata) VALUES ").
		WriteTuples(states).
		WriteString(" ON CONFLICT (ns,pkey) DO UPDATE SET val = EXCLUDED.val, kversion = EXCLUDED.kversion, metadata = EXCLUDED.metadata").
		Build()
	return query, params, nil
}

// SetStatusesValid builds an UPDATE statement that marks all transactions in
// txIDs as Valid and clears their message, so a transaction committed after a
// local discard does not keep the discard reason. It returns the SQL query string
// and the corresponding argument slice ready for execution.
func (db *VaultStore) SetStatusesValid(txIDs []driver.TxID) (string, []any, error) {
	query, params := sqlbuild.New().
		WriteString("UPDATE " + db.tables.StatusTable + " SET message = '', code = ").
		WriteParam(driver.Valid).
		WriteWhere(sqlbuild.In("tx_id", txIDs...)).
		Build()
	return query, params, nil
}

func (db *VaultStore) convertStateRows(writes driver.Writes, metaWrites driver.MetaWrites) ([]sqlbuild.Tuple, error) {
	states := make([]sqlbuild.Tuple, 0, len(writes))
	for ns, write := range writes {
		metaWrite, ok := metaWrites[ns]
		if !ok {
			metaWrite = map[driver.PKey]driver.VaultMetadataValue{}
		}
		for pkey, val := range write {
			metadata := make([]byte, 0)
			var metaVersion driver.RawVersion
			var err error
			if metaVal, ok := metaWrite[pkey]; ok {
				metadata, err = db.marshallMetadata(metaVal.Metadata)
				metaVersion = metaVal.Version
			}
			if len(metaVersion) > 0 && !bytes.Equal(val.Version, metaVersion) {
				logger.Warnf("different values passed for metadata version and data version: [%s] [%s]", metaVersion, val.Version)
			}
			if err != nil {
				return nil, errors.Wrapf(err, "failed to marshal metadata for [%s:%s]", ns, pkey)
			}
			if len(val.Raw) == 0 {
				// Deleting value
				logger.Debugf("setting version of [%s] to nil", pkey)
				val.Version = nil
			}
			states = append(states, sqlbuild.Tuple{ns, db.encodeKey(pkey), val.Raw, val.Version, metadata})
		}
	}

	for ns, metaWrite := range metaWrites {
		write, ok := writes[ns]
		if !ok {
			write = map[driver.PKey]driver.VaultValue{}
		}
		for pkey, metaVal := range metaWrite {
			if _, ok = write[pkey]; ok {
				// MetaWrites already written in the previous section
				continue
			}
			metadata, err := db.marshallMetadata(metaVal.Metadata)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to marshal metadata for [%s:%s]", ns, pkey)
			}
			states = append(states, sqlbuild.Tuple{ns, db.encodeKey(pkey), []byte{}, metaVal.Version, metadata})
		}
	}
	return states, nil
}

type dbReader interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func newTxVaultReader(newVaultReader func() (*vaultReader, releaseFunc, error)) *txVaultReader {
	return &txVaultReader{newVaultReader: newVaultReader, release: func() error { return nil }}
}

type releaseFunc func() error

type txVaultReader struct {
	once           sync.Once
	newVaultReader func() (*vaultReader, releaseFunc, error)
	vr             *vaultReader
	release        releaseFunc
	// err holds the outcome of the single open attempt. It must live on the
	// struct rather than in setVaultReader: once.Do runs only once, so a local
	// would be nil on every later call and the caller would dereference a nil vr.
	// On failure vr and release keep their initial values, so Done stays safe.
	err error
}

func (db *txVaultReader) setVaultReader() error {
	db.once.Do(func() {
		vr, release, err := db.newVaultReader()
		if err != nil {
			db.err = err
			return
		}
		db.vr, db.release = vr, release
	})
	return db.err
}

func (db *txVaultReader) GetState(ctx context.Context, namespace driver.Namespace, key driver.PKey) (*driver.VaultRead, error) {
	if err := db.setVaultReader(); err != nil {
		return nil, err
	}
	return db.vr.GetState(ctx, namespace, key)
}

func (db *txVaultReader) GetStates(ctx context.Context, namespace driver.Namespace, keys ...driver.PKey) (driver.TxStateIterator, error) {
	if err := db.setVaultReader(); err != nil {
		return nil, err
	}
	return db.vr.GetStates(ctx, namespace, keys...)
}

func (db *txVaultReader) GetStateRange(ctx context.Context, namespace driver.Namespace, startKey, endKey driver.PKey) (driver.TxStateIterator, error) {
	if err := db.setVaultReader(); err != nil {
		return nil, err
	}
	return db.vr.GetStateRange(ctx, namespace, startKey, endKey)
}

func (db *txVaultReader) GetAllStates(ctx context.Context, namespace driver.Namespace) (driver.TxStateIterator, error) {
	if err := db.setVaultReader(); err != nil {
		return nil, err
	}
	return db.vr.GetAllStates(ctx, namespace)
}

func (db *txVaultReader) GetStateMetadata(ctx context.Context, namespace driver.Namespace, key driver.PKey) (driver.Metadata, driver.RawVersion, error) {
	if err := db.setVaultReader(); err != nil {
		return nil, nil, err
	}
	return db.vr.GetStateMetadata(ctx, namespace, key)
}

func (db *txVaultReader) GetLast(ctx context.Context) (*driver.TxStatus, error) {
	if err := db.setVaultReader(); err != nil {
		return nil, err
	}
	return db.vr.GetLast(ctx)
}

func (db *txVaultReader) GetTxStatus(ctx context.Context, txID driver.TxID) (*driver.TxStatus, error) {
	if err := db.setVaultReader(); err != nil {
		return nil, err
	}
	return db.vr.GetTxStatus(ctx, txID)
}

func (db *txVaultReader) GetTxStatuses(ctx context.Context, txIDs ...driver.TxID) (driver.TxStatusIterator, error) {
	if err := db.setVaultReader(); err != nil {
		return nil, err
	}
	return db.vr.GetTxStatuses(ctx, txIDs...)
}

func (db *txVaultReader) GetAllTxStatuses(ctx context.Context, p driver.Pagination) (*driver.PageIterator[*driver.TxStatus], error) {
	if err := db.setVaultReader(); err != nil {
		return nil, err
	}
	return db.vr.GetAllTxStatuses(ctx, p)
}

func (db *txVaultReader) Done() error {
	return db.release()
}

// vaultReader runs the vault's read queries. Errors from the database are
// returned through errorWrapper, so callers can match the backend-independent
// sentinels of the storage driver package.
type vaultReader struct {
	readDB       dbReader
	encodeKey    KeyEncoder
	tables       VaultTables
	errorWrapper dbdriver.SQLErrorWrapper
	timeout      common.QueryTimeout
}

// queryRows runs query on readDB and returns an iterator over its rows decoded by
// scan, which keeps the statement's deadline until it is closed.
func queryRows[V any](ctx context.Context, db *vaultReader, query string, params []any, scan func(*sql.Rows, *V) error) (iterators.Iterator[*V], error) {
	ctx, cancel := db.timeout.Context(ctx)
	rows, err := db.readDB.QueryContext(ctx, query, params...)
	if err != nil {
		cancel()
		return nil, db.errorWrapper.WrapError(err)
	}
	return common.NewWrappedIteratorWithCancel(rows, func(v *V) error { return scan(rows, v) }, db.errorWrapper, cancel), nil
}

func scanVaultRead(rows *sql.Rows, r *driver.VaultRead) error {
	return rows.Scan(&r.Key, &r.Version, &r.Raw)
}

func (db *vaultReader) GetState(ctx context.Context, namespace driver.Namespace, key driver.PKey) (*driver.VaultRead, error) {
	it, err := db.GetStates(ctx, namespace, key)
	if err != nil {
		return nil, err
	}
	return collections.GetUnique(it)
}

func (db *vaultReader) GetStates(ctx context.Context, namespace driver.Namespace, keys ...driver.PKey) (driver.TxStateIterator, error) {
	encoded := make([]any, len(keys))
	for i, k := range keys {
		encoded[i] = db.encodeKey(k)
	}
	return db.queryState(ctx, sqlbuild.And(sqlbuild.Eq("ns", namespace), sqlbuild.In("pkey", encoded...)))
}

func (db *vaultReader) GetStateRange(ctx context.Context, namespace driver.Namespace, startKey, endKey driver.PKey) (driver.TxStateIterator, error) {
	query, params := sqlbuild.New().
		WriteString("SELECT pkey, kversion, val FROM " + db.tables.StateTable).
		WriteWhere(sqlbuild.And(sqlbuild.Eq("ns", namespace), betweenKeys("pkey", startKey, endKey, db.encodeKey))).
		WriteString(" ORDER BY pkey ASC").
		Build()

	logger.Debug(query, params)
	return queryRows(ctx, db, query, params, scanVaultRead)
}

func (db *vaultReader) GetAllStates(ctx context.Context, namespace driver.Namespace) (driver.TxStateIterator, error) {
	return db.queryState(ctx, sqlbuild.Eq("ns", namespace))
}

// betweenKeys returns a condition matching rows where col falls in the
// half-open range [start, end), with both bounds converted by encode. Empty
// start or end omits that bound; with both empty it renders the always-true
// (1=1), so every row matches.
func betweenKeys(col string, start, end driver.PKey, encode KeyEncoder) sqlbuild.Condition {
	var parts []sqlbuild.Condition
	if start != "" {
		parts = append(parts, sqlbuild.Gte(col, encode(start)))
	}
	if end != "" {
		parts = append(parts, sqlbuild.Lt(col, encode(end)))
	}
	return sqlbuild.And(parts...)
}

// queryState runs a SELECT on the state table filtered by where and returns
// an iterator over the matching rows.
func (db *vaultReader) queryState(ctx context.Context, where sqlbuild.Condition) (driver.TxStateIterator, error) {
	query, params := sqlbuild.New().
		WriteString("SELECT pkey, kversion, val FROM " + db.tables.StateTable).
		WriteWhere(where).
		Build()

	logger.Debug(query, params)
	return queryRows(ctx, db, query, params, scanVaultRead)
}

func (db *vaultReader) GetStateMetadata(ctx context.Context, namespace driver.Namespace, key driver.PKey) (driver.Metadata, driver.RawVersion, error) {
	query, params := sqlbuild.New().
		WriteString("SELECT metadata, kversion FROM " + db.tables.StateTable).
		WriteWhere(sqlbuild.And(sqlbuild.Eq("ns", namespace), sqlbuild.Eq("pkey", db.encodeKey(key)))).
		Build()
	logger.Debug(query, params)

	ctx, cancel := db.timeout.Context(ctx)
	defer cancel()
	row := db.readDB.QueryRowContext(ctx, query, params...)
	var m []byte
	var kversion driver.RawVersion
	err := row.Scan(&m, &kversion)
	if err != nil && errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, errors.Wrap(db.errorWrapper.WrapError(err), "error querying db")
	}
	meta, err := unmarshalMetadata(m)
	if err != nil {
		return meta, nil, errors.Wrap(err, "error decoding metadata")
	}

	return meta, kversion, err
}

func (db *vaultReader) GetLast(ctx context.Context) (*driver.TxStatus, error) {
	// Select the row with the highest pos that is not Busy
	it, err := db.queryStatus(ctx, sqlbuild.CondFunc(func(b *sqlbuild.Builder) {
		b.WriteString("pos=(SELECT max(pos) FROM " + db.tables.StatusTable + " WHERE code!=").
			WriteParam(driver.Busy).
			WriteString(")")
	}), pagination.None())
	if err != nil {
		return nil, err
	}
	return collections.GetUnique(it)
}

func (db *vaultReader) GetTxStatus(ctx context.Context, txID driver.TxID) (*driver.TxStatus, error) {
	it, err := db.queryStatus(ctx, sqlbuild.Eq("tx_id", txID), pagination.None())
	if err != nil {
		return nil, err
	}
	return collections.GetUnique(it)
}

func (db *vaultReader) GetTxStatuses(ctx context.Context, txIDs ...driver.TxID) (driver.TxStatusIterator, error) {
	if len(txIDs) == 0 {
		return collections.NewEmptyIterator[*driver.TxStatus](), nil
	}
	return db.queryStatus(ctx, sqlbuild.In("tx_id", txIDs...), pagination.None())
}

func (db *vaultReader) GetAllTxStatuses(ctx context.Context, p driver.Pagination) (*driver.PageIterator[*driver.TxStatus], error) {
	if p == nil {
		return nil, errors.Errorf("invalid input pagination: %+v", p)
	}

	txStatusIterator, err := db.queryStatus(ctx, nil, p)
	if err != nil {
		return nil, err
	}
	return pagination.NewPage[driver.TxStatus](txStatusIterator, p)
}

// queryStatus runs a SELECT on the status table filtered by the optional
// where condition, applies pagination, and returns an iterator over the results.
func (db *vaultReader) queryStatus(ctx context.Context, where sqlbuild.Condition, p driver.Pagination) (driver.TxStatusIterator, error) {
	pag := pagination.Paging(p)
	query, params := sqlbuild.New().
		WriteString("SELECT tx_id, code, message FROM "+db.tables.StatusTable).
		WriteWhere(where, pag.Where).
		WritePaging(pag).
		Build()
	logger.Debug(query, params)

	return queryRows(ctx, db, query, params, func(rows *sql.Rows, s *driver.TxStatus) error { return rows.Scan(&s.TxID, &s.Code, &s.Message) })
}

func (db *VaultStore) Close() error {
	return errors.Join(db.writeDB.Close(), db.readDB.Close())
}

// Data marshal/unmarshal

// protoMetadataStep is the contract step after which the vault writes metadata as protobuf.
const protoMetadataStep = 2

// metadataPageSize is the number of rows MetadataToProtobuf reads and rewrites at a time.
const metadataPageSize = 1000

// protoMetadataTag is the first byte of every protobuf VaultMetadata the vault writes: the tag
// of field 1 (entries), length-delimited, which writers always set, even for an empty map. A gob
// stream starts with a message length, never this byte for a map, so the first byte tells the
// forms apart without depending on the type ids gob assigned earlier in the process.
const protoMetadataTag = 0x0a

// marshallMetadata encodes metadata as gob until the vault's contract step 2 has run, and
// afterwards as the protobuf VaultMetadata toProtoMetadata gives. A map protobuf cannot carry
// exactly, such as one with a key that is not UTF-8, is encoded as gob in either case.
func (db *VaultStore) marshallMetadata(metadata map[string][]byte) ([]byte, error) {
	if db.readableBy.Load() >= protoMetadataStep {
		if raw, ok := toProtoMetadata(metadata); ok {
			return raw, nil
		}
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(metadata); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// toProtoMetadata returns the deterministic protobuf VaultMetadata of m if it decodes back to
// the entries of m (see sameMetadata), so that no metadata is replaced by protobuf that reads
// differently. The entries are always set, so an empty or nil map encodes as 0a 00 and reads
// back as an empty map, as its gob encoding does, not as no metadata.
func toProtoMetadata(m map[string][]byte) ([]byte, bool) {
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(&protos.VaultMetadata{Entries: &protos.Entries{Values: m}})
	if err != nil {
		return nil, false
	}
	back, err := unmarshalMetadata(raw)
	if err != nil || !sameMetadata(m, back) {
		return nil, false
	}
	return raw, true
}

// sameMetadata reports whether a and b hold the same keys with the same bytes. A nil value
// equals an empty one, as gob decodes an empty value as nil and protobuf as empty.
func sameMetadata(a, b map[string][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok || !bytes.Equal(av, bv) {
			return false
		}
	}
	return true
}

// unmarshalMetadata decodes metadata in either form the vault stores: input starting with
// protoMetadataTag is protobuf VaultMetadata, other input is gob. Empty input is no metadata
// and returns a nil map; protobuf metadata returns a non-nil map, empty for no entries.
func unmarshalMetadata(input []byte) (map[string][]byte, error) {
	if len(input) == 0 {
		return nil, nil
	}
	if input[0] == protoMetadataTag {
		var m protos.VaultMetadata
		if err := proto.Unmarshal(input, &m); err != nil {
			return nil, err
		}
		if values := m.GetEntries().GetValues(); values != nil {
			return values, nil
		}
		return map[string][]byte{}, nil
	}
	var m map[string][]byte
	err := gob.NewDecoder(bytes.NewReader(input)).Decode(&m)
	return m, err
}

// metadataRow is a row of the state table with metadata. pkey keeps the type the driver
// scanned, so that it binds back as the same SQL type: TEXT on SQLite, BYTEA on Postgres.
type metadataRow struct {
	ns       string
	pkey     any
	metadata []byte
}

// MetadataToProtobuf is the vault's contract step 2 on both dialects. It rewrites the gob
// metadata of every row as the protobuf toProtoMetadata gives, in pages of metadataPageSize
// rows in (ns, pkey) order. Each page is read in full and its rows closed before its rows are
// updated, as a connection runs one statement at a time. Rows without metadata and rows
// already in protobuf stay unchanged; rows that do not decode as gob or do not convert exactly
// stay unchanged too and are counted in one warning. A row is rewritten only if its metadata
// still holds the bytes the step read: on Postgres the step runs in READ COMMITTED and does not
// lock the state table, so a node that keeps running can commit new metadata for a row after
// the step read it. Such a row keeps the metadata that node wrote, which readers accept in
// either form, and is counted separately.
func (db *VaultStore) MetadataToProtobuf(ctx context.Context, q migrate.Execer) error {
	table := db.tables.StateTable
	read, converted, changed, failed := 0, 0, 0, 0
	// The first page has no lower bound, so that a row with the empty key is read too.
	query, args := "SELECT ns, pkey, metadata FROM "+table+" WHERE length(metadata) > 0 ORDER BY ns, pkey LIMIT $1", []any{metadataPageSize} //nolint:gosec // table name comes from configuration
	for {
		page, err := readMetadataPage(ctx, q, query, args...)
		if err != nil {
			return errors.Wrapf(err, "failed reading [%s]", table)
		}
		for _, r := range page {
			if r.metadata[0] == protoMetadataTag {
				continue
			}
			m, err := unmarshalMetadata(r.metadata)
			if err != nil {
				failed++
				continue
			}
			raw, ok := toProtoMetadata(m)
			if !ok {
				failed++
				continue
			}
			res, err := q.ExecContext(ctx, "UPDATE "+table+" SET metadata = $1 WHERE ns = $2 AND pkey = $3 AND metadata = $4", raw, r.ns, r.pkey, r.metadata) //nolint:gosec // table name comes from configuration
			if err != nil {
				return errors.Wrapf(err, "failed rewriting metadata of [%s:%v] in [%s]", r.ns, r.pkey, table)
			}
			n, err := res.RowsAffected()
			if err != nil {
				return errors.Wrapf(err, "failed rewriting metadata of [%s:%v] in [%s]", r.ns, r.pkey, table)
			}
			if n == 0 {
				changed++
				continue
			}
			converted++
		}
		read += len(page)
		logger.Infof("store [%s]: read %d row(s) with metadata, rewrote %d as protobuf, %d changed meanwhile and left as is, so far", table, read, converted, changed)
		if len(page) < metadataPageSize {
			break
		}
		last := page[len(page)-1]
		query, args = "SELECT ns, pkey, metadata FROM "+table+" WHERE (ns, pkey) > ($1, $2) AND length(metadata) > 0 ORDER BY ns, pkey LIMIT $3", []any{last.ns, last.pkey, metadataPageSize} //nolint:gosec // table name comes from configuration
	}
	if failed > 0 {
		logger.Warnf("store [%s]: %d row(s) of metadata do not convert to protobuf and stay gob", table, failed)
	}
	logger.Infof("store [%s]: rewrote the metadata of %d row(s) as protobuf; %d row(s) changed meanwhile and were left as is", table, converted, changed)
	return nil
}

// readMetadataPage runs query and returns all its rows, closed.
func readMetadataPage(ctx context.Context, q migrate.Execer, query string, args ...any) ([]metadataRow, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var page []metadataRow
	for rows.Next() {
		var r metadataRow
		if err := rows.Scan(&r.ns, &r.pkey, &r.metadata); err != nil {
			return nil, err
		}
		page = append(page, r)
	}
	return page, rows.Err()
}
