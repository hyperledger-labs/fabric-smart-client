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
	"encoding/json"
	"fmt"
	"maps"
	"sort"
	"sync"
	"testing"
	"unicode/utf8"

	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/protos"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
)

// KeyDataStore is the get/exists/put surface the envelope, endorsement-transaction
// and metadata stores share under their own method names.
type KeyDataStore struct {
	Get    func(context.Context, string) ([]byte, error)
	Exists func(context.Context, string) (bool, error)
	Put    func(context.Context, string, []byte) error
}

// TestKeyDataStore checks s against a real database. A put value is read back
// and an unknown key is absent. A second put under the same key with a
// different value keeps the first value and, when strict, returns an error
// wrapping driver.ErrConflict, or nil otherwise. When strict, putting the
// stored value again returns nil, empty data is refused, and of two concurrent
// puts of different values under one key exactly one succeeds.
func TestKeyDataStore(t *testing.T, s KeyDataStore, strict bool) {
	t.Helper()
	ctx := t.Context()
	// Not valid UTF-8 and containing a NUL, so a column that is not BYTEA rejects it.
	first := []byte{0x00, 0xff, 'f'}

	require.NoError(t, s.Put(ctx, "tx1", first))
	err := s.Put(ctx, "tx1", []byte("second"))
	if strict {
		require.ErrorIs(t, err, driver.ErrConflict)
		require.NoError(t, s.Put(ctx, "tx1", first), "putting the stored value again is not an error")
	} else {
		require.NoError(t, err, "putting an existing key is not an error")
	}

	got, err := s.Get(ctx, "tx1")
	require.NoError(t, err)
	assert.Equal(t, first, got, "the first put wins")

	exists, err := s.Exists(ctx, "tx1")
	require.NoError(t, err)
	assert.True(t, exists)

	got, err = s.Get(ctx, "unknown")
	require.NoError(t, err)
	assert.Empty(t, got, "an unknown key returns nothing")

	exists, err = s.Exists(ctx, "unknown")
	require.NoError(t, err)
	assert.False(t, exists)

	if !strict {
		return
	}
	require.Error(t, s.Put(ctx, "empty", nil), "a write-once store refuses empty data, which Exists reports as absent")
	exists, err = s.Exists(ctx, "empty")
	require.NoError(t, err)
	assert.False(t, exists)

	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Go(func() { errs[i] = s.Put(ctx, "race", []byte{byte(i)}) })
	}
	wg.Wait()
	if errs[0] != nil {
		errs[0], errs[1] = errs[1], errs[0]
	}
	require.NoError(t, errs[0], "one of two concurrent puts succeeds")
	require.ErrorIs(t, errs[1], driver.ErrConflict, "the other conflicts")
}

// EndorseTxContract is an endorse-tx store on a table that existed before the store was
// versioned, so that its required contract step is pending, with the means to inspect the
// table, run the step and open the store as a node does.
type EndorseTxContract struct {
	// Store reads and writes the table without opening it through Migrate.
	Store KeyDataStore
	// DB writes rows directly into Table, bypassing the store.
	DB    *sql.DB
	Table string
	// Contract runs the store's pending contract step on Store, as node migrate --contract does.
	Contract func() error
	// Open opens the store through the driver's constructor, as a node does.
	Open func() error
	// Step is the Go function of the contract step, for running it on a chosen Execer.
	Step func(context.Context, migrate.Execer) error
	// Concurrent, if set, writes rows from outside the step's transaction while it runs; if
	// nil, such writes go through the step's own transaction, as SQLite has one writer.
	Concurrent *sql.DB
}

// requireContract checks that open refuses a store while its required contract step 2 is
// pending, runs the step with contract, and checks that open succeeds afterwards.
func requireContract(t *testing.T, open, contract func() error) {
	t.Helper()
	err := open()
	require.ErrorIs(t, err, migrate.ErrContractPending)
	require.ErrorContains(t, err, "step 2 [")
	require.NoError(t, contract())
	require.NoError(t, open())
}

// insertRows writes rows into c's table directly, as a binary that stored JSON did before the
// contract step.
func insertRows(t *testing.T, c EndorseTxContract, rows map[string][]byte) {
	t.Helper()
	ctx := t.Context()
	tx, err := c.DB.BeginTx(ctx, nil)
	require.NoError(t, err)
	for k, v := range rows {
		_, err := tx.ExecContext(ctx, "INSERT INTO "+c.Table+" (key, data) VALUES ($1, $2)", k, v) //nolint:gosec // test table name
		require.NoError(t, err)
	}
	require.NoError(t, tx.Commit())
}

// endorseTxJSON returns the JSON of a Fabric transaction in the form protos.JSONFromEnvelope
// gives, so that converting it to an envelope and back yields the same bytes. Odd i carries a
// channel key, even i none. The transient map has several keys, so its order matters to a
// marshaller that is not deterministic.
func endorseTxJSON(t *testing.T, i int) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"TTxID": fmt.Sprintf("tx%d", i), "TChannel": "ch", "TNetwork": "net", "RWSet": []byte(fmt.Sprintf("rws%d", i)),
		"TTransient": map[string][]byte{"a": []byte("1"), "b": []byte("2"), "c": []byte("3")},
	})
	require.NoError(t, err)
	wrapper := map[string]any{"Type": int(fdriver.EndorserTransaction), "Raw": raw}
	if i%2 == 1 {
		wrapper["Key"] = fdriver.ChannelKey{Platform: "generic", Channel: "ch", GenesisHash: []byte("genesis")}
	}
	raw, err = json.Marshal(wrapper)
	require.NoError(t, err)
	env, err := protos.EnvelopeFromJSON(raw)
	require.NoError(t, err)
	raw, err = protos.JSONFromEnvelope(env)
	require.NoError(t, err)
	return raw
}

// bytesTx has the fields of a Fabric transaction (core/generic/transaction.Transaction) in the
// order its Bytes marshals them, so bytesTxJSON gives the bytes Bytes gives without importing
// the transaction core into the store.
type bytesTx struct {
	TCreator           []byte
	TNonce             []byte
	TTxID              string
	TNetwork           string
	TChannel           string
	TChaincode         string
	TChaincodeVersion  string
	TFunction          string
	TParameters        [][]byte
	RWSet              []byte
	TTransient         map[string][]byte
	TProposal          *pb.Proposal
	TSignedProposal    *pb.SignedProposal
	TProposalResponses []*pb.ProposalResponse
}

// bytesTxJSON returns the bytes WrappedTransaction.Bytes gives for tx, a Fabric endorser
// transaction whose channel key cannot be read.
func bytesTxJSON(t *testing.T, tx bytesTx) []byte {
	t.Helper()
	raw, err := json.Marshal(&tx)
	require.NoError(t, err)
	raw, err = json.Marshal(&struct {
		Type fdriver.TransactionType
		Raw  []byte
	}{fdriver.EndorserTransaction, raw})
	require.NoError(t, err)
	return raw
}

// unsignedBytesTx is transaction i as an unsigned Fabric transaction without parameters and
// with an empty read-write set holds it, as Bytes gives it: nil TParameters, an empty RWSet and
// a null TProposal. Odd i has no transient map. The JSON an envelope reads back as differs from
// these bytes in exactly those fields, which the store does not compare.
func unsignedBytesTx(i int) bytesTx {
	tx := bytesTx{
		TCreator: []byte("creator"), TNonce: fmt.Appendf(nil, "nonce%d", i), TTxID: fmt.Sprintf("btx%d", i),
		TNetwork: "net", TChannel: "ch", TChaincode: "cc", TFunction: "invoke", RWSet: []byte{},
	}
	if i%2 == 0 {
		tx.TTransient = map[string][]byte{"a": []byte("1")}
	}
	return tx
}

// lossyTxJSON is a transaction that converts to an envelope which reads back as a different
// transaction: its parameter that is null reads back as an empty parameter. The store keeps
// it JSON.
func lossyTxJSON(t *testing.T) []byte {
	t.Helper()
	tx := unsignedBytesTx(0)
	tx.TTxID = "lossy"
	tx.TParameters = [][]byte{[]byte("a"), nil}
	return bytesTxJSON(t, tx)
}

// unregisteredTxJSON is a transaction with a channel key of a platform without a registered
// transaction format, as a binary without that platform's transaction core reads it. The
// store keeps it JSON.
var unregisteredTxJSON = []byte(`{"TTxID":"unreg","TChannel":"ch","TTransient":{},"Key":{"platform":"unregistered","channel":"ch","genesisHash":"Z2VuZXNpcw=="}}`)

// keylessFlatTxJSON is a transaction that is not wrapped in a Fabric SerializedTransaction and
// carries no channel key, as other platforms write them. The store cannot tell its platform, so
// it stays JSON.
var keylessFlatTxJSON = []byte(`{"TTxID":"flat","TChannel":"ch","TTransient":{}}`)

// withField returns the JSON object raw with field set to v.
func withField(t *testing.T, raw []byte, field string, v any) []byte {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	m[field] = v
	out, err := json.Marshal(m)
	require.NoError(t, err)
	return out
}

// envelopeOf returns the envelope protos.EnvelopeFromJSON gives for raw, checking that a
// second conversion gives the same bytes.
func envelopeOf(t *testing.T, raw []byte) []byte {
	t.Helper()
	env, err := protos.EnvelopeFromJSON(raw)
	require.NoError(t, err)
	again, err := protos.EnvelopeFromJSON(raw)
	require.NoError(t, err)
	require.Equal(t, env, again, "the conversion is deterministic")
	return env
}

// beforeFirstExec is a migrate.Execer that calls before once, ahead of its first ExecContext:
// after a contract step read its first page and before it rewrites any row of it.
type beforeFirstExec struct {
	migrate.Execer
	before func()
}

func (e *beforeFirstExec) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if e.before != nil {
		before := e.before
		e.before = nil
		before()
	}
	return e.Execer.ExecContext(ctx, query, args...)
}

// runStepWithConcurrentWrite runs step in a transaction on db and, after it read its first
// page and before its first rewrite, runs the UPDATE query with args on concurrent, or on the
// step's transaction if concurrent is nil.
func runStepWithConcurrentWrite(t *testing.T, db, concurrent *sql.DB, step func(context.Context, migrate.Execer) error, query string, args ...any) {
	t.Helper()
	ctx := t.Context()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	var w migrate.Execer = tx
	if concurrent != nil {
		w = concurrent
	}
	q := &beforeFirstExec{Execer: tx, before: func() {
		res, err := w.ExecContext(ctx, query, args...)
		require.NoError(t, err)
		n, err := res.RowsAffected()
		require.NoError(t, err)
		require.Equal(t, int64(1), n, "the concurrent write hits one row")
	}}
	require.NoError(t, step(ctx, q))
	require.NoError(t, tx.Commit())
}

// TestEndorseTxStepKeepsConcurrentWrite inserts three JSON transactions and runs the contract
// step while another writer replaces the last one after the step read it. That row keeps the
// other writer's bytes; the others are converted. On Postgres, the concurrent write needs its
// own connection, so c.Concurrent requires WriteDB to allow at least 2 open connections.
func TestEndorseTxStepKeepsConcurrentWrite(t *testing.T, c EndorseTxContract) {
	t.Helper()
	ctx := t.Context()
	insertRows(t, c, map[string][]byte{"k1": endorseTxJSON(t, 0), "k2": endorseTxJSON(t, 1), "k3": endorseTxJSON(t, 2)})
	newer := endorseTxJSON(t, 9)
	runStepWithConcurrentWrite(t, c.DB, c.Concurrent, c.Step,
		"UPDATE "+c.Table+" SET data = $1 WHERE key = $2", newer, "k3")

	for i, k := range []string{"k1", "k2"} {
		got, err := c.Store.Get(ctx, k)
		require.NoError(t, err)
		require.Equal(t, envelopeOf(t, endorseTxJSON(t, i)), got, "[%s]", k)
	}
	got, err := c.Store.Get(ctx, "k3")
	require.NoError(t, err)
	require.Equal(t, newer, got, "the concurrent write is kept")
}

// TestEndorseTxContractStep fills c's table with 2500 JSON transactions, more than two pages
// of the step, plus transactions as Transaction.Bytes gives them, a non-transaction value,
// JSON that is not a transaction ({}, an unknown object, a transaction with a field FSC does
// not know, malformed JSON), a transaction without a channel key that does not tell its
// platform, one of a platform without a registered format, one whose envelope reads back as a
// different transaction, and an envelope, and runs the contract step. Every transaction then
// holds the envelope protos.EnvelopeFromJSON gives for it, which reads back as the same
// transaction (byte for byte for the first 2500); the other rows are unchanged, and writing a
// transaction's JSON again is not a conflict.
func TestEndorseTxContractStep(t *testing.T, c EndorseTxContract) {
	t.Helper()
	ctx := t.Context()
	const n = 2500
	others := map[string][]byte{
		"etx-key": []byte("etx"),
		"bad":     []byte("{not a transaction"),
		"empty":   []byte("{}"),
		"foo":     []byte(`{"foo":1}`),
		"extra":   withField(t, endorseTxJSON(t, n+2), "Extra", 1),
		"keyless": keylessFlatTxJSON,
		"unreg":   unregisteredTxJSON,
		"lossy":   lossyTxJSON(t),
		"env":     envelopeOf(t, endorseTxJSON(t, n)),
		"":        endorseTxJSON(t, n+1), // the lowest possible key
	}
	// Transactions as Bytes gives them, not in the form an envelope reads back as.
	const nBytes = 4
	bytesRows := make([][]byte, nBytes)
	for i := range bytesRows {
		bytesRows[i] = bytesTxJSON(t, unsignedBytesTx(i))
	}
	rows := maps.Clone(others)
	for i := range n {
		rows[fmt.Sprintf("k%05d", i)] = endorseTxJSON(t, i)
	}
	for i, v := range bytesRows {
		rows[fmt.Sprintf("b%d", i)] = v
	}
	insertRows(t, c, rows)

	requireContract(t, c.Open, c.Contract)

	for i, raw := range bytesRows {
		got, err := c.Store.Get(ctx, fmt.Sprintf("b%d", i))
		require.NoError(t, err)
		require.Equal(t, envelopeOf(t, raw), got, "Bytes row %d is converted", i)
		back, err := protos.JSONFromEnvelope(got)
		require.NoError(t, err)
		require.NotEqual(t, raw, back, "Bytes row %d is not in the form an envelope reads back as", i)
		require.True(t, protos.SameTransactionJSON(raw, back), "Bytes row %d", i)
		require.NoError(t, c.Store.Put(ctx, fmt.Sprintf("b%d", i), raw), "the JSON of a converted Bytes row is the same value")
	}
	for i := range n {
		want := endorseTxJSON(t, i)
		got, err := c.Store.Get(ctx, fmt.Sprintf("k%05d", i))
		require.NoError(t, err)
		require.Equal(t, envelopeOf(t, want), got, "row %d", i)
		back, err := protos.JSONFromEnvelope(got)
		require.NoError(t, err)
		require.Equal(t, want, back, "row %d", i)
	}
	for k, v := range others {
		got, err := c.Store.Get(ctx, k)
		require.NoError(t, err)
		if k == "" {
			assert.Equal(t, envelopeOf(t, v), got, "the lowest key is converted")
			continue
		}
		assert.Equal(t, v, got, "row [%s] is unchanged", k)
	}
	require.NoError(t, c.Store.Put(ctx, "k00007", endorseTxJSON(t, 7)), "the JSON of a converted row is the same value")
}

// TestEndorseTxWritesAfterContract inserts JSON before c's contract step and writes JSON after
// it. The step converts the JSON inserted before, and writing that JSON again is not a
// conflict; JSON written after is stored as its envelope, and values that are not convertible
// JSON as given.
func TestEndorseTxWritesAfterContract(t *testing.T, c EndorseTxContract) {
	t.Helper()
	ctx := t.Context()
	before := endorseTxJSON(t, 1)
	insertRows(t, c, map[string][]byte{"before": before})

	requireContract(t, c.Open, c.Contract)

	got, err := c.Store.Get(ctx, "before")
	require.NoError(t, err)
	require.Equal(t, envelopeOf(t, before), got)
	require.NoError(t, c.Store.Put(ctx, "before", before), "the JSON converts to the envelope the step wrote")

	after := endorseTxJSON(t, 2)
	require.NoError(t, c.Store.Put(ctx, "after", after))
	got, err = c.Store.Get(ctx, "after")
	require.NoError(t, err)
	require.Equal(t, envelopeOf(t, after), got, "JSON is stored as its envelope")

	for k, v := range map[string][]byte{
		"env":     envelopeOf(t, endorseTxJSON(t, 3)),
		"bad":     []byte("{not a transaction"),
		"etx":     []byte("etx"),
		"empty":   []byte("{}"),
		"foo":     []byte(`{"foo":1}`),
		"keyless": keylessFlatTxJSON,
	} {
		require.NoError(t, c.Store.Put(ctx, k, v))
		got, err := c.Store.Get(ctx, k)
		require.NoError(t, err)
		require.Equal(t, v, got, "[%s] is stored as given", k)
	}
}

// VaultContract is a vault on tables that existed before the vault was versioned, so that its
// required contract step is pending, with the means to inspect the state table, run the step
// and open the vault as a node does.
type VaultContract struct {
	// Store reads and writes the tables without opening them through Migrate.
	Store cdriver.VaultStore
	// DB reads the state table directly, bypassing the store.
	DB    *sql.DB
	Table string
	// Contract runs the vault's pending contract step on Store, as node migrate --contract does.
	Contract func() error
	// Open opens the vault through the driver's constructor, as a node does.
	Open func() error
	// Step is the Go function of the contract step, for running it on a chosen Execer.
	Step func(context.Context, migrate.Execer) error
	// Concurrent, if set, writes rows from outside the step's transaction while it runs; if
	// nil, such writes go through the step's own transaction, as SQLite has one writer.
	Concurrent *sql.DB
}

// vaultMetadataOf returns the metadata TestVaultMetadataContractStep writes under key i.
func vaultMetadataOf(i int) map[string][]byte {
	return map[string][]byte{"a": fmt.Appendf(nil, "one%d", i), "b": []byte("two"), "c": fmt.Appendf(nil, "%d", i)}
}

// requireSameMetadata checks that got holds the entries of want, treating a nil value as
// empty, as gob decodes an empty value as nil.
func requireSameMetadata(t *testing.T, want, got map[string][]byte, msg string) {
	t.Helper()
	require.Len(t, got, len(want), msg)
	for k, v := range want {
		g, ok := got[k]
		require.True(t, ok, "%s: entry [%q]", msg, k)
		require.Equal(t, string(v), string(g), "%s: entry [%q]", msg, k)
	}
}

// storeGobMetadata stores each key of kv in ns through c.Store, with the value ns+"/"+key, and
// replaces its metadata with the gob encoding of kv[key], the form a vault wrote before its
// contract step.
func storeGobMetadata(t *testing.T, c VaultContract, ns cdriver.Namespace, kv map[cdriver.PKey]map[string][]byte) {
	t.Helper()
	ctx := t.Context()
	writes, metaWrites := cdriver.Writes{ns: {}}, cdriver.MetaWrites{ns: {}}
	for k := range kv {
		writes[ns][k] = cdriver.VaultValue{Raw: []byte(ns + "/" + k), Version: []byte("v")}
		metaWrites[ns][k] = cdriver.VaultMetadataValue{Version: []byte("v")}
	}
	require.NoError(t, c.Store.Store(ctx, nil, writes, metaWrites))
	tx, err := c.DB.BeginTx(ctx, nil)
	require.NoError(t, err)
	// val identifies the row in either dialect; pkey is TEXT on SQLite and BYTEA on Postgres.
	for k, m := range kv {
		_, err := tx.ExecContext(ctx, "UPDATE "+c.Table+" SET metadata = $1 WHERE val = $2", gobMetadata(t, m), []byte(ns+"/"+k)) //nolint:gosec // test table name
		require.NoError(t, err)
	}
	require.NoError(t, tx.Commit())
}

// rawMetadata returns the metadata column of every row of ns in c's state table.
func rawMetadata(t *testing.T, c VaultContract, ns string) [][]byte {
	t.Helper()
	rows, err := c.DB.QueryContext(t.Context(), "SELECT metadata FROM "+c.Table+" WHERE ns = $1", ns) //nolint:gosec // test table name
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var out [][]byte
	for rows.Next() {
		var m []byte
		require.NoError(t, rows.Scan(&m))
		out = append(out, m)
	}
	require.NoError(t, rows.Err())
	return out
}

// TestVaultMetadataContractStep writes 2500 keys with gob metadata, more than two pages of the
// step, including keys with U+0000 and U+10FFFF, keys in a second namespace, metadata with an
// empty value, an empty map, a metadata key protobuf refuses, and a key without metadata, and
// runs the contract step. GetStateMetadata then returns the same metadata for every key; every
// row converts except the refused key, which stays gob; new writes are protobuf, and a new
// write of a metadata key protobuf refuses fails.
func TestVaultMetadataContractStep(t *testing.T, c VaultContract) {
	t.Helper()
	ctx := t.Context()
	const n = 2500
	// edge sorts last on the first page (index 999), so the keyset cursor of the second page
	// carries a key with U+0000.
	edge := cdriver.PKey("k00995\x00edge")
	keys := []cdriver.PKey{"\x00Token\x00alice\x00", "end" + string(utf8.MaxRune), "", edge}
	for i := range n - len(keys) {
		keys = append(keys, fmt.Sprintf("k%05d", i))
	}
	sorted := append([]cdriver.PKey(nil), keys...)
	sort.Strings(sorted)
	require.Equal(t, edge, sorted[metadataPageSize-1], "the edge key ends the first page")
	want := map[cdriver.Namespace]map[cdriver.PKey]map[string][]byte{"ns": {}, "ns-b": {
		"k1":        {"a": []byte("b1")},
		"empty-val": {"e": []byte{}, "f": []byte("f")},
		"empty-map": {},
		"bad-key":   {"\xff": []byte("v")},
	}}
	for i, k := range keys {
		want["ns"][k] = vaultMetadataOf(i)
	}
	for ns, kv := range want {
		storeGobMetadata(t, c, ns, kv)
	}
	require.NoError(t, c.Store.Store(ctx, nil, cdriver.Writes{"ns-b": {"no-meta": {Raw: []byte("val"), Version: []byte("v")}}}, nil))
	for _, ns := range []string{"ns", "ns-b"} {
		for _, m := range rawMetadata(t, c, ns) {
			if len(m) > 0 {
				require.NotEqual(t, byte(0x0a), m[0], "gob before the step")
			}
		}
	}

	requireContract(t, c.Open, c.Contract)

	for ns, kv := range want {
		for k, m := range kv {
			got, _, err := c.Store.GetStateMetadata(ctx, ns, k)
			require.NoError(t, err)
			requireSameMetadata(t, m, got, fmt.Sprintf("[%s:%q]", ns, k))
		}
	}
	got, _, err := c.Store.GetStateMetadata(ctx, "ns-b", "empty-map")
	require.NoError(t, err)
	require.NotNil(t, got, "an empty map stays an empty map, not no metadata")
	require.Contains(t, rawMetadata(t, c, "ns-b"), []byte{0x0a, 0x00}, "the step converts an empty map")
	got, _, err = c.Store.GetStateMetadata(ctx, "ns-b", "no-meta")
	require.NoError(t, err)
	require.Empty(t, got, "no metadata stays no metadata")
	gobRows := 0
	for _, ns := range []string{"ns", "ns-b"} {
		for _, m := range rawMetadata(t, c, ns) {
			if len(m) > 0 && m[0] != 0x0a {
				gobRows++
			}
		}
	}
	require.Equal(t, 1, gobRows, "only the row protobuf refuses stays gob")

	require.NoError(t, c.Store.Store(ctx, nil, cdriver.Writes{"new": {"k": {Raw: []byte("val"), Version: []byte("v")}}},
		cdriver.MetaWrites{"new": {"k": {Metadata: map[string][]byte{"x": []byte("y")}, Version: []byte("v")}}}))
	raw := rawMetadata(t, c, "new")
	require.Len(t, raw, 1)
	require.Equal(t, byte(0x0a), raw[0][0], "new writes are protobuf")
	got, _, err = c.Store.GetStateMetadata(ctx, "new", "k")
	require.NoError(t, err)
	require.Equal(t, map[string][]byte{"x": []byte("y")}, got)

	err = c.Store.Store(ctx, nil, nil, cdriver.MetaWrites{"new": {"bad": {Metadata: map[string][]byte{"\xff": []byte("v")}, Version: []byte("v")}}})
	require.ErrorContains(t, err, `[new:bad]`)
	require.ErrorContains(t, err, `metadata key ["\xff"] is not valid UTF-8`)
}

// gobMetadata returns the gob encoding of m, the form a vault wrote before its contract step.
func gobMetadata(t *testing.T, m map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, gob.NewEncoder(&buf).Encode(m))
	return buf.Bytes()
}

// TestVaultMetadataStepKeepsConcurrentWrite writes three keys with gob metadata and runs the
// contract step while another writer replaces the metadata of the last key after the step read
// it. That key keeps the other writer's metadata; the other keys are converted. On Postgres,
// the concurrent write needs its own connection, so c.Concurrent requires WriteDB to allow at
// least 2 open connections.
func TestVaultMetadataStepKeepsConcurrentWrite(t *testing.T, c VaultContract) {
	t.Helper()
	ctx := t.Context()
	keys := []cdriver.PKey{"k1", "k2", "k3"}
	kv := map[cdriver.PKey]map[string][]byte{}
	for i, k := range keys {
		kv[k] = vaultMetadataOf(i)
	}
	storeGobMetadata(t, c, "ns", kv)

	newer := map[string][]byte{"z": []byte("written meanwhile")}
	runStepWithConcurrentWrite(t, c.DB, c.Concurrent, c.Step,
		"UPDATE "+c.Table+" SET metadata = $1 WHERE val = $2", gobMetadata(t, newer), []byte("ns/k3"))

	for i, k := range keys[:2] {
		got, _, err := c.Store.GetStateMetadata(ctx, "ns", k)
		require.NoError(t, err)
		require.Equal(t, vaultMetadataOf(i), got, "[%s]", k)
	}
	got, _, err := c.Store.GetStateMetadata(ctx, "ns", "k3")
	require.NoError(t, err)
	require.Equal(t, newer, got, "the concurrent write is kept")
	proto, other := 0, 0
	for _, m := range rawMetadata(t, c, "ns") {
		if m[0] == protoMetadataTag {
			proto++
		} else {
			require.Equal(t, gobMetadata(t, newer), m, "the concurrent write is kept byte for byte")
			other++
		}
	}
	require.Equal(t, []int{2, 1}, []int{proto, other})
}
