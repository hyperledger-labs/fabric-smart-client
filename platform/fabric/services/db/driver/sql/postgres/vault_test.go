/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/compose"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	testing2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/testing"
	postgres2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
)

// newPostgresVaultStore starts a postgres container and returns a VaultStore
// whose schema has been created against it. The container is terminated through
// testing.Cleanup.
//
// It goes through NewPersistenceWithOpts rather than buildVaultStore so that
// CreateSchema runs the same way it does in production.
func newPostgresVaultStore(tb testing.TB) *VaultStore {
	tb.Helper()
	return newPostgresVaultStoreWithParams(tb, "")
}

// newPostgresVaultStoreWithParams is newPostgresVaultStore with params appended
// to the connection string, which pgx sends as session parameters on every
// pooled connection.
func newPostgresVaultStoreWithParams(tb testing.TB, params string) *VaultStore {
	tb.Helper()

	terminate, pgConnStr, err := postgres2.StartPostgres(tb.Context(), postgres2.ConfigFromEnv(), nil)
	require.NoError(tb, err)
	tb.Cleanup(terminate)

	cp := postgres2.NewConfigProvider(testing2.MockConfig(postgres2.Config{
		DataSource: pgConnStr + " " + params,
	}))
	store, err := NewPersistenceWithOpts(cp, postgres2.NewDbProvider(), "", NewVaultStore)
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = store.Close() })

	return store
}

// TestPostgresVaultStore covers Store: the statuses it marks busy then valid,
// and the states and metadata it upserts, all inside one transaction. The name
// carries "Postgres" so CI's --run/--skip gate gives it to the container suite
// only.
func TestPostgresVaultStore(t *testing.T) {
	t.Parallel()

	store := newPostgresVaultStore(t)
	ctx := t.Context()

	txIDs := []driver.TxID{"tx1", "tx2"}
	writes := driver.Writes{
		"ns1": {
			"k1": driver.VaultValue{Raw: []byte("v1"), Version: []byte("ver1")},
			"k2": driver.VaultValue{Raw: []byte("v2"), Version: []byte("ver2")},
		},
	}
	metaWrites := driver.MetaWrites{
		"ns1": {
			"k1": driver.VaultMetadataValue{
				Version:  []byte("ver1"),
				Metadata: map[string][]byte{"m": []byte("meta1")},
			},
		},
	}

	require.NoError(t, store.Store(ctx, txIDs, writes, metaWrites))

	r, err := store.NewGlobalLockVaultReader(ctx)
	require.NoError(t, err)

	// Store leaves every tx it was given valid, not busy.
	for _, txID := range txIDs {
		status, err := r.GetTxStatus(ctx, txID)
		require.NoError(t, err)
		require.NotNil(t, status)
		require.Equal(t, driver.Valid, status.Code)
	}

	state, err := r.GetState(ctx, "ns1", "k1")
	require.NoError(t, err)
	require.NotNil(t, state)
	require.Equal(t, driver.RawValue("v1"), state.Raw)
	require.Equal(t, driver.RawVersion("ver1"), state.Version)

	state, err = r.GetState(ctx, "ns1", "k2")
	require.NoError(t, err)
	require.NotNil(t, state)
	require.Equal(t, driver.RawValue("v2"), state.Raw)

	meta, version, err := r.GetStateMetadata(ctx, "ns1", "k1")
	require.NoError(t, err)
	require.Equal(t, driver.RawVersion("ver1"), version)
	require.Equal(t, []byte("meta1"), meta["m"])
	require.NoError(t, r.Done())
}

// Store is called with no statuses and no writes on transactions that change
// nothing, so the empty case must commit cleanly rather than fail on an empty
// query.
func TestPostgresVaultStoreEmpty(t *testing.T) {
	t.Parallel()

	store := newPostgresVaultStore(t)
	require.NoError(t, store.Store(t.Context(), nil, nil, nil))
}

// Statuses and states are written independently: a call carrying only txIDs
// records them without any state, and a call carrying only writes records
// state without touching statuses.
func TestPostgresVaultStoreStatusesOnly(t *testing.T) {
	t.Parallel()

	store := newPostgresVaultStore(t)
	ctx := t.Context()

	require.NoError(t, store.Store(ctx, []driver.TxID{"txOnly"}, nil, nil))

	r, err := store.NewGlobalLockVaultReader(ctx)
	require.NoError(t, err)
	status, err := r.GetTxStatus(ctx, "txOnly")
	require.NoError(t, err)
	require.NotNil(t, status)
	require.Equal(t, driver.Valid, status.Code)
	require.NoError(t, r.Done())
}

func TestPostgresVaultStoreWritesOnly(t *testing.T) {
	t.Parallel()

	store := newPostgresVaultStore(t)
	ctx := t.Context()

	require.NoError(t, store.Store(ctx, nil, driver.Writes{
		"ns2": {"k9": driver.VaultValue{Raw: []byte("v9"), Version: []byte("ver9")}},
	}, nil))

	r, err := store.NewGlobalLockVaultReader(ctx)
	require.NoError(t, err)
	state, err := r.GetState(ctx, "ns2", "k9")
	require.NoError(t, err)
	require.NotNil(t, state)
	require.Equal(t, driver.RawValue("v9"), state.Raw)
	require.NoError(t, r.Done())
}

// Store upserts: a second write to the same key replaces the value rather than
// failing on the state table's (ns, pkey) primary key.
func TestPostgresVaultStoreUpsert(t *testing.T) {
	t.Parallel()

	store := newPostgresVaultStore(t)
	ctx := t.Context()

	write := func(raw, version string) error {
		return store.Store(ctx, nil, driver.Writes{
			"ns3": {"k1": driver.VaultValue{Raw: []byte(raw), Version: []byte(version)}},
		}, nil)
	}
	require.NoError(t, write("first", "ver1"))
	require.NoError(t, write("second", "ver2"))

	r, err := store.NewGlobalLockVaultReader(ctx)
	require.NoError(t, err)
	state, err := r.GetState(ctx, "ns3", "k1")
	require.NoError(t, err)
	require.NotNil(t, state)
	require.Equal(t, driver.RawValue("second"), state.Raw)
	require.Equal(t, driver.RawVersion("ver2"), state.Version)
	require.NoError(t, r.Done())
}

// Storing the same transactions twice, as the committer does when it commits a
// block again after a connection lost during COMMIT, succeeds and leaves one
// valid status row per transaction and the same states.
func TestPostgresVaultStoreRecommitIsIdempotent(t *testing.T) {
	t.Parallel()

	store := newPostgresVaultStore(t)
	ctx := t.Context()

	txIDs := []driver.TxID{"tx1", "tx2"}
	writes := driver.Writes{"ns": {"k": driver.VaultValue{Raw: []byte("v"), Version: []byte("ver")}}}
	metaWrites := driver.MetaWrites{"ns": {"k": driver.VaultMetadataValue{
		Version:  []byte("ver"),
		Metadata: map[string][]byte{"m": []byte("meta")},
	}}}
	require.NoError(t, store.Store(ctx, txIDs, writes, metaWrites))
	require.NoError(t, store.Store(ctx, txIDs, writes, metaWrites))

	count := func(query string, args ...any) int {
		var n int
		require.NoError(t, store.writeDB.QueryRowContext(ctx, query, args...).Scan(&n))
		return n
	}
	require.Equal(t, 2, count("SELECT count(*) FROM "+store.tables.StatusTable))
	require.Equal(t, 2, count("SELECT count(*) FROM "+store.tables.StatusTable+" WHERE code = $1", driver.Valid))
	require.Equal(t, 1, count("SELECT count(*) FROM "+store.tables.StateTable))

	r, err := store.NewGlobalLockVaultReader(ctx)
	require.NoError(t, err)
	state, err := r.GetState(ctx, "ns", "k")
	require.NoError(t, err)
	require.Equal(t, driver.RawValue("v"), state.Raw)
	meta, _, err := r.GetStateMetadata(ctx, "ns", "k")
	require.NoError(t, err)
	require.Equal(t, []byte("meta"), meta["m"])
	require.NoError(t, r.Done())
}

// A Store whose connection is terminated mid-transaction returns
// dbdriver.ErrConnectionLost, which the committer retries, and the next Store
// succeeds on a fresh connection.
func TestPostgresVaultStoreConnectionLost(t *testing.T) {
	t.Parallel()

	store := newPostgresVaultStore(t)
	ctx := t.Context()

	// An uncommitted insert of tx1 holds its row lock, so Store blocks on it
	// inside its transaction until its backend is terminated.
	lock, err := store.writeDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = lock.Rollback() }()
	_, err = lock.ExecContext(ctx, "INSERT INTO "+store.tables.StatusTable+" (tx_id, code) VALUES ('tx1', $1)", driver.Busy)
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() { done <- store.Store(ctx, []driver.TxID{"tx1"}, nil, nil) }()

	require.Eventually(t, func() bool {
		var n int
		err := store.writeDB.QueryRowContext(ctx,
			"SELECT count(pg_terminate_backend(pid)) FROM pg_stat_activity WHERE wait_event_type = 'Lock' AND datname = current_database()",
		).Scan(&n)
		return err == nil && n > 0
	}, 10*time.Second, 20*time.Millisecond)

	err = <-done
	require.ErrorIs(t, err, dbdriver.ErrConnectionLost)
	require.NoError(t, lock.Rollback())

	require.NoError(t, store.Store(ctx, []driver.TxID{"tx1"}, nil, nil))
}

// CreateSchema is idempotent — every constructor runs it unless
// SkipCreateTable is set, so a second node opening the same database must not
// fail on the existing tables.
func TestPostgresVaultStoreCreateSchemaIdempotent(t *testing.T) {
	t.Parallel()

	store := newPostgresVaultStore(t)
	require.NoError(t, store.CreateSchema())
	require.NoError(t, store.CreateSchema())
}

// legacyEscape escapes a key the way a TEXT pkey column stores it: U+0000 as
// "?0?" and U+10FFFF as "?1?", with "?" itself left as is.
var legacyEscape = strings.NewReplacer("\x00", "?0?", string(utf8.MaxRune), "?1?").Replace

// createTextKeyTable replaces the store's state table with one whose pkey column
// is TEXT and fills it with rows, keyed by unescaped key, stored escaped. Each
// row's value is its key and its version is "v".
func createTextKeyTable(t *testing.T, store *VaultStore, ns string, keys []driver.PKey) {
	t.Helper()
	ctx := t.Context()
	table := store.tables.StateTable
	_, err := store.writeDB.ExecContext(ctx, "DROP TABLE "+table)
	require.NoError(t, err)
	_, err = store.writeDB.ExecContext(ctx, "CREATE TABLE "+table+` (
		ns TEXT NOT NULL,
		pkey TEXT NOT NULL,
		val BYTEA,
		kversion BYTEA,
		metadata BYTEA,
		PRIMARY KEY (pkey, ns)
	)`)
	require.NoError(t, err)
	for _, k := range keys {
		_, err = store.writeDB.ExecContext(ctx, "INSERT INTO "+table+" (ns, pkey, val, kversion, metadata) VALUES ($1, $2, $3, $4, $5)",
			ns, legacyEscape(k), []byte(k), []byte("v"), []byte{})
		require.NoError(t, err)
	}
}

func requirePkeyType(t *testing.T, store *VaultStore, want string) {
	t.Helper()
	got, err := store.pkeyType(store.writeDB)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

// textKeys covers every escape the TEXT encoding uses, keys without escapes,
// non-ASCII text, and a literal "?".
var textKeys = []driver.PKey{
	compose.CreateCompositeKeyOrPanic(&strings.Builder{}, "Token", "alice"),
	compose.CreateCompositeKeyOrPanic(&strings.Builder{}, "Token", "alice", "xyz"),
	compose.CreateCompositeKeyOrPanic(&strings.Builder{}, "Token", "alice", "123"),
	compose.CreateCompositeKeyOrPanic(&strings.Builder{}, "Token", "bob"),
	"plain",
	"mid\x00dle",
	"what?is?this",
	"über",
	"end" + string(utf8.MaxRune),
}

// CreateSchema converts a TEXT pkey column to BYTEA in place: every row reads
// back under its unescaped key, partial-composite-key range scans find every
// child, and keys that the TEXT encoding conflated are distinct afterwards.
func TestPostgresVaultStoreConvertsTextKeys(t *testing.T) {
	t.Parallel()

	store := newPostgresVaultStore(t)
	ctx := t.Context()
	createTextKeyTable(t, store, "ns", textKeys)
	requirePkeyType(t, store, "text")

	require.NoError(t, store.CreateSchema())
	requirePkeyType(t, store, "bytea")
	requirePrimaryKey(t, store, "ns,pkey")
	// Converting and reordering are one-offs: a second CreateSchema finds the
	// new layout and does nothing.
	require.NoError(t, store.CreateSchema())

	for _, k := range textKeys {
		read, err := store.GetState(ctx, "ns", k)
		require.NoError(t, err)
		require.NotNil(t, read, "key [%q]", k)
		require.Equal(t, k, read.Key)
		require.Equal(t, driver.RawValue(k), read.Raw)
		require.Equal(t, driver.RawVersion("v"), read.Version)
	}

	start, end, err := compose.CreateRangeKeysForPartialCompositeKey("Token", "alice")
	require.NoError(t, err)
	it, err := store.GetStateRange(ctx, "ns", start, end)
	require.NoError(t, err)
	reads, err := collections.ReadAll(it)
	require.NoError(t, err)
	got := make([]driver.PKey, len(reads))
	for i, r := range reads {
		got[i] = r.Key
	}
	require.ElementsMatch(t, textKeys[:3], got)

	// The plain key spelling out the composite key's escapes is now its own row.
	forged := legacyEscape(textKeys[0])
	require.NoError(t, store.Store(ctx, nil, driver.Writes{"ns": {forged: {Raw: []byte("forged"), Version: []byte("v2")}}}, nil))
	read, err := store.GetState(ctx, "ns", textKeys[0])
	require.NoError(t, err)
	require.Equal(t, driver.RawValue(textKeys[0]), read.Raw)
	read, err = store.GetState(ctx, "ns", forged)
	require.NoError(t, err)
	require.Equal(t, driver.RawValue("forged"), read.Raw)
}

// GetStateRange returns keys sorted in byte order even when the planner reads
// the state table in physical order, as it may for a range covering much of
// the table.
func TestPostgresVaultStoreRangeIsSorted(t *testing.T) {
	t.Parallel()

	store := newPostgresVaultStoreWithParams(t, "enable_indexscan=off enable_bitmapscan=off")
	ctx := t.Context()
	// One Store per key, so the rows are laid out in this order.
	for _, k := range []driver.PKey{"k3", "k2", "k111", "k1"} {
		require.NoError(t, store.Store(ctx, nil, driver.Writes{"ns": {k: {Raw: []byte(k), Version: []byte("v")}}}, nil))
	}

	it, err := store.GetStateRange(ctx, "ns", "k1", "k3")
	require.NoError(t, err)
	reads, err := collections.ReadAll(it)
	require.NoError(t, err)
	got := make([]driver.PKey, len(reads))
	for i, r := range reads {
		got[i] = r.Key
	}
	require.Equal(t, []driver.PKey{"k1", "k111", "k2"}, got)
}

// Replicas sharing a database may open a TEXT-keyed vault at the same time.
// The test holds a lock on the state table until every replica has read the
// TEXT type and is queued for the conversion lock, so all but the first find
// the table already converted once they get it. Each CreateSchema succeeds and
// the table is converted once.
func TestPostgresVaultStoreConvertsTextKeysConcurrently(t *testing.T) {
	t.Parallel()

	store := newPostgresVaultStore(t)
	ctx := t.Context()
	table := store.tables.StateTable
	createTextKeyTable(t, store, "ns", textKeys)

	holder, err := store.writeDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = holder.ExecContext(ctx, "LOCK TABLE "+table+" IN ACCESS SHARE MODE")
	require.NoError(t, err)

	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Go(func() { errs[i] = store.CreateSchema() })
	}
	require.Eventually(t, func() bool {
		var waiting int
		err := store.writeDB.QueryRowContext(ctx, "SELECT count(*) FROM pg_locks WHERE relation = $1::regclass AND NOT granted", table).Scan(&waiting)
		return err == nil && waiting == len(errs)
	}, 30*time.Second, 10*time.Millisecond, "every replica queues for the conversion lock")
	require.NoError(t, holder.Rollback())
	wg.Wait()

	for _, err := range errs {
		require.NoError(t, err)
	}
	requirePkeyType(t, store, "bytea")
	requirePrimaryKey(t, store, "ns,pkey")

	read, err := store.GetState(ctx, "ns", textKeys[0])
	require.NoError(t, err)
	require.NotNil(t, read)
	require.Equal(t, driver.RawValue(textKeys[0]), read.Raw)
}

// A pkey type other than TEXT or BYTEA is not converted: CreateSchema fails and
// leaves the table as it is.
func TestPostgresVaultStoreCreateSchemaRejectsOtherKeyType(t *testing.T) {
	t.Parallel()

	store := newPostgresVaultStore(t)
	_, err := store.writeDB.ExecContext(t.Context(), "ALTER TABLE "+store.tables.StateTable+" ALTER COLUMN pkey TYPE VARCHAR(255) USING encode(pkey, 'escape')")
	require.NoError(t, err)

	err = store.CreateSchema()
	require.ErrorContains(t, err, "stores pkey as [character varying(255)], expected [bytea]")
	requirePkeyType(t, store, "character varying(255)")
}

// recreateStateTable replaces the store's state table with a BYTEA-keyed one
// declaring primaryKey, so tests can start from a layout CreateSchema no longer
// creates.
func recreateStateTable(t *testing.T, store *VaultStore, primaryKey string) {
	t.Helper()
	table := store.tables.StateTable
	_, err := store.writeDB.ExecContext(t.Context(), "DROP TABLE "+table)
	require.NoError(t, err)
	_, err = store.writeDB.ExecContext(t.Context(), "CREATE TABLE "+table+` (
		ns TEXT NOT NULL,
		pkey BYTEA NOT NULL,
		val BYTEA,
		kversion BYTEA,
		metadata BYTEA,
		`+primaryKey+`
	)`)
	require.NoError(t, err)
}

func requirePrimaryKey(t *testing.T, store *VaultStore, want string) {
	t.Helper()
	_, got, err := store.primaryKey(store.writeDB)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

// CreateSchema reorders a BYTEA table whose primary key is (pkey, ns). The
// constraint name is not the default, rows in two namespaces that share keys
// stay in the namespace they were written to, and a second call does nothing.
func TestPostgresVaultStoreReordersPrimaryKey(t *testing.T) {
	t.Parallel()

	store := newPostgresVaultStore(t)
	ctx := t.Context()
	recreateStateTable(t, store, "CONSTRAINT vault_state_old_pk PRIMARY KEY (pkey, ns)")
	require.NoError(t, store.Store(ctx, nil, driver.Writes{
		"ns-a": {
			"k2":                 {Raw: []byte("a2"), Version: []byte("v")},
			"k1":                 {Raw: []byte("a1"), Version: []byte("v")},
			"\x00Token\x00alice": {Raw: []byte("tok"), Version: []byte("v")},
		},
		"ns-b": {
			"k1": {Raw: []byte("b1"), Version: []byte("v")},
			"k2": {Raw: []byte("b2"), Version: []byte("v")},
		},
	}, nil))

	require.NoError(t, store.CreateSchema())
	requirePrimaryKey(t, store, "ns,pkey")
	var left int
	require.NoError(t, store.writeDB.QueryRowContext(ctx,
		"SELECT count(*) FROM pg_constraint WHERE conname = 'vault_state_old_pk'").Scan(&left))
	require.Zero(t, left)
	require.NoError(t, store.CreateSchema())

	read, err := store.GetState(ctx, "ns-a", "\x00Token\x00alice")
	require.NoError(t, err)
	require.NotNil(t, read)
	require.Equal(t, driver.RawValue("tok"), read.Raw)
	read, err = store.GetState(ctx, "ns-b", "k1")
	require.NoError(t, err)
	require.Equal(t, driver.RawValue("b1"), read.Raw)

	it, err := store.GetStateRange(ctx, "ns-a", "k1", "k3")
	require.NoError(t, err)
	reads, err := collections.ReadAll(it)
	require.NoError(t, err)
	require.Len(t, reads, 2)
	require.Equal(t, []driver.PKey{"k1", "k2"}, []driver.PKey{reads[0].Key, reads[1].Key})
}

// A primary key other than (pkey, ns) or (ns, pkey) is not rewritten.
func TestPostgresVaultStoreRejectsOtherPrimaryKey(t *testing.T) {
	t.Parallel()

	store := newPostgresVaultStore(t)
	ctx := t.Context()
	recreateStateTable(t, store, "PRIMARY KEY (pkey)")
	table := store.tables.StateTable
	_, err := store.writeDB.ExecContext(ctx, "INSERT INTO "+table+" (ns, pkey, val, kversion) VALUES ($1, $2, $3, $4)",
		"ns", []byte("k1"), []byte("v"), []byte("v"))
	require.NoError(t, err)

	err = store.CreateSchema()
	require.ErrorContains(t, err, "primary key is (pkey), expected (ns, pkey)")
	requirePrimaryKey(t, store, "pkey")
	var n int
	require.NoError(t, store.writeDB.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&n))
	require.Equal(t, 1, n)
}

// GetStateRange scans the (ns, pkey) index from the namespace and does not
// sort.
func TestPostgresVaultStoreRangePlanUsesNamespaceKey(t *testing.T) {
	t.Parallel()

	store := newPostgresVaultStore(t)
	ctx := t.Context()
	for _, ns := range []string{"ns-a", "ns-b"} {
		for _, k := range []string{"k1", "k2", "k3"} {
			require.NoError(t, store.Store(ctx, nil, driver.Writes{
				ns: {k: {Raw: []byte(k), Version: []byte("v")}},
			}, nil))
		}
	}

	tx, err := store.writeDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	_, err = tx.Exec("SET LOCAL enable_seqscan = off")
	require.NoError(t, err)
	_, err = tx.Exec("SET LOCAL enable_bitmapscan = off")
	require.NoError(t, err)
	rows, err := tx.Query("EXPLAIN SELECT pkey, kversion, val FROM "+store.tables.StateTable+
		" WHERE (ns = $1 AND (pkey >= $2 AND pkey < $3)) ORDER BY pkey ASC", "ns-a", "k1", "k3")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var plan strings.Builder
	for rows.Next() {
		var line string
		require.NoError(t, rows.Scan(&line))
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	require.NoError(t, rows.Err())
	// The (pkey, ns) index also serves this query, with ns as a filter inside
	// the index condition; only (ns, pkey) puts ns first.
	require.Contains(t, plan.String(), "Index Cond: ((ns = ")
	require.NotContains(t, plan.String(), "Sort")
}

// Replicas sharing a BYTEA table keyed (pkey, ns) all start: the first to get
// the lock reorders it, and the others find (ns, pkey) under the lock.
func TestPostgresVaultStoreReordersPrimaryKeyConcurrently(t *testing.T) {
	t.Parallel()

	store := newPostgresVaultStore(t)
	ctx := t.Context()
	table := store.tables.StateTable
	recreateStateTable(t, store, "PRIMARY KEY (pkey, ns)")
	require.NoError(t, store.Store(ctx, nil, driver.Writes{"ns": {"k1": {Raw: []byte("v"), Version: []byte("v")}}}, nil))

	holder, err := store.writeDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = holder.ExecContext(ctx, "LOCK TABLE "+table+" IN ACCESS SHARE MODE")
	require.NoError(t, err)

	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Go(func() { errs[i] = store.CreateSchema() })
	}
	require.Eventually(t, func() bool {
		var waiting int
		err := store.writeDB.QueryRowContext(ctx, "SELECT count(*) FROM pg_locks WHERE relation = $1::regclass AND NOT granted", table).Scan(&waiting)
		return err == nil && waiting == len(errs)
	}, 30*time.Second, 10*time.Millisecond, "every replica queues for the reorder lock")
	require.NoError(t, holder.Rollback())
	wg.Wait()

	for _, err := range errs {
		require.NoError(t, err)
	}
	requirePrimaryKey(t, store, "ns,pkey")
	read, err := store.GetState(ctx, "ns", "k1")
	require.NoError(t, err)
	require.NotNil(t, read)
	require.Equal(t, driver.RawValue("v"), read.Raw)
}

// A table without a primary key is reported as such and left unchanged.
func TestPostgresVaultStoreRejectsMissingPrimaryKey(t *testing.T) {
	t.Parallel()

	store := newPostgresVaultStore(t)
	recreateStateTable(t, store, "CHECK (true)")

	require.ErrorContains(t, store.CreateSchema(), "has no primary key")
	_, _, err := store.primaryKey(store.writeDB)
	require.ErrorContains(t, err, "has no primary key")
}
