/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sql_test

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	fpostgres "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/postgres"
	fsqlite "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/sqlite"
	drivertesting "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/testing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// fixtureVersions are the releases under testdata whose databases HEAD must
// open and migrate. testdata/README.md describes how they are generated.
var fixtureVersions = []string{"v0.26.0", "v0.27.0"}

// vaultKeys are the keys testdata/generate.go writes to the vault.
var vaultKeys = []driver.PKey{"plain", "\x00Token\x00alice\x00", "end" + string(utf8.MaxRune)}

// readFixture returns the content of file in the fixture directory of version,
// after checking it against the directory's SHA256SUMS.
func readFixture(t *testing.T, version, file string) []byte {
	t.Helper()
	dir := filepath.Join("testdata", version)
	sums, err := os.Open(filepath.Join(dir, "SHA256SUMS"))
	require.NoError(t, err)
	defer func() { _ = sums.Close() }()
	want := map[string]string{}
	for s := bufio.NewScanner(sums); s.Scan(); {
		sum, name, ok := strings.Cut(s.Text(), "  ")
		require.True(t, ok, "malformed SHA256SUMS line [%s]", s.Text())
		want[name] = sum
	}
	raw, err := os.ReadFile(filepath.Join(dir, file))
	require.NoError(t, err)
	got := sha256.Sum256(raw)
	require.Equal(t, want[file], hex.EncodeToString(got[:]), "%s/%s does not match SHA256SUMS; regenerate it with testdata/generate.sh", version, file)
	return raw
}

// fixtureStores is one store of every FSC store type, opened at HEAD.
type fixtureStores struct {
	kvs interface {
		GetState(context.Context, driver.Namespace, driver.PKey) (driver.UnversionedValue, error)
	}
	bind interface {
		GetLongTerm(context.Context, view.Identity) (view.Identity, error)
	}
	sign interface {
		FilterExistingSigners(context.Context, ...view.Identity) ([]view.Identity, error)
	}
	aud interface {
		GetAuditInfo(context.Context, view.Identity) ([]byte, error)
	}
	etx interface {
		GetEndorseTx(context.Context, string) ([]byte, error)
	}
	meta interface {
		GetMetadata(context.Context, string) ([]byte, error)
	}
	env interface {
		GetEnvelope(context.Context, string) ([]byte, error)
	}
	vault driver.VaultStore
}

// checkRows reads back every row testdata/generate.go wrote.
func checkRows(t *testing.T, s fixtureStores) {
	t.Helper()
	ctx := t.Context()
	for _, ns := range []driver.Namespace{"ns-a", "ns-b"} {
		got, err := s.kvs.GetState(ctx, ns, "k1")
		require.NoError(t, err)
		assert.Equal(t, driver.UnversionedValue("kvs-"+ns), got)
	}
	lt, err := s.bind.GetLongTerm(ctx, view.Identity("ephemeral"))
	require.NoError(t, err)
	assert.Equal(t, view.Identity("long-term"), lt)
	signers, err := s.sign.FilterExistingSigners(ctx, view.Identity("signer"), view.Identity("other"))
	require.NoError(t, err)
	assert.Equal(t, []view.Identity{view.Identity("signer")}, signers)
	info, err := s.aud.GetAuditInfo(ctx, view.Identity("audited"))
	require.NoError(t, err)
	assert.Equal(t, []byte("audit-info"), info)
	for key, get := range map[string]func(context.Context, string) ([]byte, error){"etx": s.etx.GetEndorseTx, "meta": s.meta.GetMetadata, "env": s.env.GetEnvelope} {
		got, err := get(ctx, key+"-key")
		require.NoError(t, err)
		assert.Equal(t, []byte(key), got)
	}
	for _, k := range vaultKeys {
		read, err := s.vault.GetState(ctx, "ns", k)
		require.NoError(t, err)
		require.NotNil(t, read, "vault key [%q]", k)
		assert.Equal(t, driver.RawValue("val-"+strings.ToValidUTF8(k, "?")), read.Raw)
	}
	status, err := s.vault.GetTxStatus(ctx, "tx1")
	require.NoError(t, err)
	assert.Equal(t, driver.Valid, status.Code)
}

// requireLatest checks that every store in s is recorded at its latest step.
func requireLatest(t *testing.T, db *sql.DB, s fixtureStores) {
	t.Helper()
	for _, store := range []any{s.kvs, s.bind, s.sign, s.aud, s.etx, s.meta, s.env, s.vault} {
		v, ok := store.(migrate.Versioned)
		require.True(t, ok, "%T is not migrate.Versioned", store)
		var version int
		require.NoError(t, db.QueryRowContext(t.Context(), "SELECT version FROM fsc_schema_versions WHERE store = $1", v.StoreName()).Scan(&version))
		assert.Equal(t, len(v.Steps()), version, v.StoreName())
	}
}

// TestFixturesSQLite opens the SQLite database every release under testdata
// wrote, migrates every store, and reads back every row.
func TestFixturesSQLite(t *testing.T) {
	t.Parallel()
	for _, version := range fixtureVersions {
		t.Run(version, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "fsc.sqlite")
			require.NoError(t, os.WriteFile(path, readFixture(t, version, "fsc.sqlite"), 0o600))

			db, err := sql.Open("sqlite", "file:"+path)
			require.NoError(t, err)
			var n int
			require.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM sqlite_master WHERE type IN ('trigger', 'view')").Scan(&n))
			require.Zero(t, n, "the fixture must hold no triggers or views: they run SQL when the test opens it")
			require.NoError(t, db.Close())

			cp := sqlite.NewConfigProvider(drivertesting.MockConfig(sqlite.Config{DataSource: "file:" + path}))
			dbp := sqlite.NewDbProvider()
			var s fixtureStores
			s.kvs = must(sqlite.NewPersistenceWithOpts(cp, dbp, "", sqlite.NewKeyValueStore))(t)
			s.bind = must(sqlite.NewPersistenceWithOpts(cp, dbp, "", sqlite.NewBindingStore, "default"))(t)
			s.sign = must(sqlite.NewPersistenceWithOpts(cp, dbp, "", sqlite.NewSignerInfoStore, "default"))(t)
			s.aud = must(sqlite.NewPersistenceWithOpts(cp, dbp, "", sqlite.NewAuditInfoStore, "default"))(t)
			s.etx = must(fsqlite.NewPersistenceWithOpts(cp, dbp, "", fsqlite.NewEndorseTxStore, "default"))(t)
			s.meta = must(fsqlite.NewPersistenceWithOpts(cp, dbp, "", fsqlite.NewMetadataStore, "default"))(t)
			s.env = must(fsqlite.NewPersistenceWithOpts(cp, dbp, "", fsqlite.NewEnvelopeStore, "default"))(t)
			s.vault = must(fsqlite.NewPersistenceWithOpts(cp, dbp, "", fsqlite.NewVaultStore, "net", "ch"))(t)
			checkRows(t, s)

			rw, err := dbp.Get(sqlite.Opts{DataSource: "file:" + path})
			require.NoError(t, err)
			for _, table := range []string{"fsc_kvs", "fsc_net__ch_vstate"} {
				cols, err := sqlite.PrimaryKeyColumns(rw.ReadDB, table)
				require.NoError(t, err)
				assert.Equal(t, "ns,pkey", cols, table)
			}
			requireLatest(t, rw.ReadDB, s)
		})
	}
}

// pgTable mirrors the layout and rows testdata/generate.go records per table.
type pgTable struct {
	Columns []struct {
		Name    string  `json:"name"`
		Type    string  `json:"type"`
		NotNull bool    `json:"notNull"`
		Default *string `json:"default"`
	} `json:"columns"`
	Constraints []string    `json:"constraints"`
	Indexes     []string    `json:"indexes"`
	Rows        [][]*string `json:"rows"`
}

// restorePostgres creates the tables of fixture in db with their columns,
// constraints and indexes, and inserts their rows.
func restorePostgres(t *testing.T, db *sql.DB, fixture map[string]*pgTable) {
	t.Helper()
	ctx := t.Context()
	names := make([]string, 0, len(fixture))
	for name := range fixture {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		table := fixture[name]
		defs := make([]string, 0, len(table.Columns)+len(table.Constraints))
		for _, c := range table.Columns {
			def := c.Name + " " + c.Type
			if seq, ok := sequenceOf(c.Default); ok {
				_, err := db.ExecContext(ctx, "CREATE SEQUENCE "+seq)
				require.NoError(t, err, seq)
			}
			if c.Default != nil {
				def += " DEFAULT " + *c.Default
			}
			if c.NotNull {
				def += " NOT NULL"
			}
			defs = append(defs, def)
		}
		defs = append(defs, table.Constraints...)
		_, err := db.ExecContext(ctx, "CREATE TABLE "+name+" ("+strings.Join(defs, ", ")+")")
		require.NoError(t, err, name)
		for _, idx := range table.Indexes {
			_, err := db.ExecContext(ctx, idx)
			require.NoError(t, err, idx)
		}
		for _, row := range table.Rows {
			cols := make([]string, len(row))
			vals := make([]string, len(row))
			args := make([]any, 0, len(row))
			for i, v := range row {
				c := table.Columns[i]
				cols[i] = c.Name
				if v == nil {
					vals[i] = "NULL"
					continue
				}
				args = append(args, *v)
				p := "$" + strconv.Itoa(len(args))
				if strings.HasPrefix(c.Type, "bytea") {
					vals[i] = "decode(" + p + ", 'hex')"
				} else {
					vals[i] = "convert_from(decode(" + p + ", 'hex'), 'UTF8')::" + c.Type
				}
			}
			_, err := db.ExecContext(ctx, "INSERT INTO "+name+" ("+strings.Join(cols, ", ")+") VALUES ("+strings.Join(vals, ", ")+")", args...) //nolint:gosec // names come from the checked fixture
			require.NoError(t, err, name)
		}
	}
	// SERIAL columns keep counting after the restored rows.
	for _, name := range names {
		for _, c := range fixture[name].Columns {
			if c.Default != nil && strings.HasPrefix(*c.Default, "nextval(") {
				_, err := db.ExecContext(ctx, "SELECT setval(pg_get_serial_sequence($1, $2), coalesce(max("+c.Name+"), 1)) FROM "+name, name, c.Name) //nolint:gosec // names come from the checked fixture
				require.NoError(t, err)
			}
		}
	}
}

// sequenceOf returns the sequence a nextval('seq'::regclass) default reads.
func sequenceOf(def *string) (string, bool) {
	if def == nil {
		return "", false
	}
	seq, ok := strings.CutPrefix(*def, "nextval('")
	if !ok {
		return "", false
	}
	seq, _, ok = strings.Cut(seq, "'")
	return seq, ok
}

// TestPostgresFixtures restores the Postgres database every release under
// testdata wrote, migrates every store, and reads back every row.
func TestPostgresFixtures(t *testing.T) {
	t.Parallel()
	for _, version := range fixtureVersions {
		t.Run(version, func(t *testing.T) {
			t.Parallel()
			fixture := map[string]*pgTable{}
			require.NoError(t, json.Unmarshal(readFixture(t, version, "fsc.postgres.json"), &fixture))

			terminate, ds, err := postgres.StartPostgres(t.Context(), postgres.ConfigFromEnv(), nil)
			require.NoError(t, err)
			t.Cleanup(terminate)
			dbp := postgres.NewDbProvider()
			rw, err := dbp.Get(postgres.Opts{DataSource: ds})
			require.NoError(t, err)
			restorePostgres(t, rw.WriteDB, fixture)

			cp := postgres.NewConfigProvider(drivertesting.MockConfig(postgres.Config{DataSource: ds}))
			var s fixtureStores
			s.kvs = must(postgres.NewPersistenceWithOpts(cp, dbp, "", postgres.NewKeyValueStore))(t)
			s.bind = must(postgres.NewPersistenceWithOpts(cp, dbp, "", postgres.NewBindingStore, "default"))(t)
			s.sign = must(postgres.NewPersistenceWithOpts(cp, dbp, "", postgres.NewSignerInfoStore, "default"))(t)
			s.aud = must(postgres.NewPersistenceWithOpts(cp, dbp, "", postgres.NewAuditInfoStore, "default"))(t)
			s.etx = must(fpostgres.NewPersistenceWithOpts(cp, dbp, "", fpostgres.NewEndorseTxStore, "default"))(t)
			s.meta = must(fpostgres.NewPersistenceWithOpts(cp, dbp, "", fpostgres.NewMetadataStore, "default"))(t)
			s.env = must(fpostgres.NewPersistenceWithOpts(cp, dbp, "", fpostgres.NewEnvelopeStore, "default"))(t)
			s.vault = must(fpostgres.NewPersistenceWithOpts(cp, dbp, "", fpostgres.NewVaultStore, "net", "ch"))(t)
			checkRows(t, s)

			for _, table := range []string{"fsc_kvs", "fsc_net__ch_vstate"} {
				_, cols, err := postgres.PrimaryKey(rw.ReadDB, table)
				require.NoError(t, err)
				assert.Equal(t, "ns,pkey", cols, table)
			}
			requireLatest(t, rw.ReadDB, s)
		})
	}
}

// must returns a function that fails t if err is set and returns v otherwise.
func must[V any](v V, err error) func(*testing.T) V {
	return func(t *testing.T) V {
		t.Helper()
		require.NoError(t, err)
		return v
	}
}
