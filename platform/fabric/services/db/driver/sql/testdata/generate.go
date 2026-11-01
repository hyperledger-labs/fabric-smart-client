//go:build ignore

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// generate writes the storage fixtures of one FSC release: it opens every FSC
// store type through that release's drivers, writes the rows in fixtureRows,
// and saves the SQLite file and the Postgres layout and rows. generate.sh runs
// it against the release; see README.md.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	fpostgres "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/postgres"
	fsqlite "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/sqlite"
	drivertesting "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/testing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// Network and channel of the vault, the table-name parameters a Fabric node
// passes for one channel.
const network, channel = "net", "ch"

// vaultKeys includes U+0000 and U+10FFFF, so the fixtures check that both
// round-trip through the BYTEA pkey column of every release.
var vaultKeys = []driver.PKey{"plain", "\x00Token\x00alice\x00", "end" + string(utf8.MaxRune)}

func main() {
	if len(os.Args) != 2 {
		fail(fmt.Errorf("usage: generate <output dir>"))
	}
	out := os.Args[1]
	ctx := context.Background()

	sqlitePath := filepath.Join(out, "fsc.sqlite")
	_ = os.Remove(sqlitePath)
	cp := sqlite.NewConfigProvider(drivertesting.MockConfig(sqlite.Config{DataSource: "file:" + sqlitePath}))
	dbp := sqlite.NewDbProvider()
	fail(write(ctx,
		func() (stores, error) {
			var s stores
			var err error
			if s.kvs, err = sqlite.NewPersistenceWithOpts(cp, dbp, "", sqlite.NewKeyValueStore); err != nil {
				return s, err
			}
			if s.bind, err = sqlite.NewPersistenceWithOpts(cp, dbp, "", sqlite.NewBindingStore, "default"); err != nil {
				return s, err
			}
			if s.sign, err = sqlite.NewPersistenceWithOpts(cp, dbp, "", sqlite.NewSignerInfoStore, "default"); err != nil {
				return s, err
			}
			if s.aud, err = sqlite.NewPersistenceWithOpts(cp, dbp, "", sqlite.NewAuditInfoStore, "default"); err != nil {
				return s, err
			}
			if s.etx, err = fsqlite.NewPersistenceWithOpts(cp, dbp, "", fsqlite.NewEndorseTxStore, "default"); err != nil {
				return s, err
			}
			if s.meta, err = fsqlite.NewPersistenceWithOpts(cp, dbp, "", fsqlite.NewMetadataStore, "default"); err != nil {
				return s, err
			}
			if s.env, err = fsqlite.NewPersistenceWithOpts(cp, dbp, "", fsqlite.NewEnvelopeStore, "default"); err != nil {
				return s, err
			}
			s.vault, err = fsqlite.NewPersistenceWithOpts(cp, dbp, "", fsqlite.NewVaultStore, network, channel)
			return s, err
		}))
	db, err := sql.Open("sqlite", "file:"+sqlitePath)
	fail(err)
	// Move the WAL into the main file and leave a self-contained file.
	for _, stmt := range []string{"PRAGMA wal_checkpoint(TRUNCATE)", "PRAGMA journal_mode=DELETE", "VACUUM"} {
		_, err = db.ExecContext(ctx, stmt)
		fail(err)
	}
	fail(db.Close())

	terminate, ds, err := postgres.StartPostgres(ctx, postgres.ConfigFromEnv(), nil)
	fail(err)
	defer terminate()
	pcp := postgres.NewConfigProvider(drivertesting.MockConfig(postgres.Config{DataSource: ds}))
	pdbp := postgres.NewDbProvider()
	fail(write(ctx,
		func() (stores, error) {
			var s stores
			var err error
			if s.kvs, err = postgres.NewPersistenceWithOpts(pcp, pdbp, "", postgres.NewKeyValueStore); err != nil {
				return s, err
			}
			if s.bind, err = postgres.NewPersistenceWithOpts(pcp, pdbp, "", postgres.NewBindingStore, "default"); err != nil {
				return s, err
			}
			if s.sign, err = postgres.NewPersistenceWithOpts(pcp, pdbp, "", postgres.NewSignerInfoStore, "default"); err != nil {
				return s, err
			}
			if s.aud, err = postgres.NewPersistenceWithOpts(pcp, pdbp, "", postgres.NewAuditInfoStore, "default"); err != nil {
				return s, err
			}
			if s.etx, err = fpostgres.NewPersistenceWithOpts(pcp, pdbp, "", fpostgres.NewEndorseTxStore, "default"); err != nil {
				return s, err
			}
			if s.meta, err = fpostgres.NewPersistenceWithOpts(pcp, pdbp, "", fpostgres.NewMetadataStore, "default"); err != nil {
				return s, err
			}
			if s.env, err = fpostgres.NewPersistenceWithOpts(pcp, pdbp, "", fpostgres.NewEnvelopeStore, "default"); err != nil {
				return s, err
			}
			s.vault, err = fpostgres.NewPersistenceWithOpts(pcp, pdbp, "", fpostgres.NewVaultStore, network, channel)
			return s, err
		}))
	pg, err := sql.Open("pgx", ds)
	fail(err)
	dump, err := dumpPostgres(ctx, pg)
	fail(err)
	raw, err := json.MarshalIndent(dump, "", "  ")
	fail(err)
	fail(os.WriteFile(filepath.Join(out, "fsc.postgres.json"), append(raw, '\n'), 0o644))
}

// stores holds one store of every FSC store type.
type stores struct {
	kvs interface {
		SetState(context.Context, driver.Namespace, driver.PKey, driver.UnversionedValue) error
		BeginUpdate() error
		Commit() error
	}
	bind  interface{ PutBindings(context.Context, view.Identity, ...view.Identity) error }
	sign  interface{ PutSigner(context.Context, view.Identity) error }
	aud   interface{ PutAuditInfo(context.Context, view.Identity, []byte) error }
	etx   interface{ PutEndorseTx(context.Context, string, []byte) error }
	meta  interface{ PutMetadata(context.Context, string, []byte) error }
	env   interface{ PutEnvelope(context.Context, string, []byte) error }
	vault interface {
		Store(context.Context, []driver.TxID, driver.Writes, driver.MetaWrites) error
		Close() error
	}
}

// write opens the stores and writes the fixture rows the test reads back.
func write(ctx context.Context, open func() (stores, error)) error {
	s, err := open()
	if err != nil {
		return err
	}
	if err := s.kvs.BeginUpdate(); err != nil {
		return err
	}
	for _, ns := range []driver.Namespace{"ns-a", "ns-b"} {
		if err := s.kvs.SetState(ctx, ns, "k1", []byte("kvs-"+ns)); err != nil {
			return err
		}
	}
	if err := s.kvs.Commit(); err != nil {
		return err
	}
	if err := s.bind.PutBindings(ctx, view.Identity("long-term"), view.Identity("ephemeral")); err != nil {
		return err
	}
	if err := s.sign.PutSigner(ctx, view.Identity("signer")); err != nil {
		return err
	}
	if err := s.aud.PutAuditInfo(ctx, view.Identity("audited"), []byte("audit-info")); err != nil {
		return err
	}
	if err := s.etx.PutEndorseTx(ctx, "etx-key", []byte("etx")); err != nil {
		return err
	}
	if err := s.meta.PutMetadata(ctx, "meta-key", []byte("meta")); err != nil {
		return err
	}
	if err := s.env.PutEnvelope(ctx, "env-key", []byte("env")); err != nil {
		return err
	}
	writes := driver.Writes{"ns": {}}
	for _, k := range vaultKeys {
		writes["ns"][k] = driver.VaultValue{Raw: []byte("val-" + strings.ToValidUTF8(string(k), "?")), Version: []byte("v1")}
	}
	if err := s.vault.Store(ctx, []driver.TxID{"tx1"}, writes, nil); err != nil {
		return err
	}
	// Every store shares one pool per data source; closing the vault closes it.
	return s.vault.Close()
}

// pgTable is the layout and the rows of one Postgres table. Values are hex
// encoded; a nil value is SQL NULL.
type pgTable struct {
	Columns     []pgColumn  `json:"columns"`
	Constraints []string    `json:"constraints"`
	Indexes     []string    `json:"indexes"`
	Rows        [][]*string `json:"rows"`
}

type pgColumn struct {
	Name    string  `json:"name"`
	Type    string  `json:"type"`
	NotNull bool    `json:"notNull"`
	Default *string `json:"default"`
}

// dumpPostgres returns every table of the public schema, keyed by name.
func dumpPostgres(ctx context.Context, db *sql.DB) (map[string]*pgTable, error) {
	tables := map[string]*pgTable{}
	rows, err := db.QueryContext(ctx, `
		SELECT c.relname, a.attname, format_type(a.atttypid, a.atttypmod), a.attnotnull, pg_get_expr(d.adbin, d.adrelid)
		FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped
		LEFT JOIN pg_attrdef d ON d.adrelid = c.oid AND d.adnum = a.attnum
		WHERE n.nspname = 'public' AND c.relkind = 'r'
		ORDER BY c.relname, a.attnum`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var table string
		var col pgColumn
		if err := rows.Scan(&table, &col.Name, &col.Type, &col.NotNull, &col.Default); err != nil {
			return nil, err
		}
		if tables[table] == nil {
			tables[table] = &pgTable{}
		}
		tables[table].Columns = append(tables[table].Columns, col)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for name, t := range tables {
		if t.Constraints, err = queryStrings(ctx, db, "SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid = $1::regclass ORDER BY contype, conname", name); err != nil {
			return nil, err
		}
		if t.Indexes, err = queryStrings(ctx, db, "SELECT indexdef FROM pg_indexes WHERE schemaname = 'public' AND tablename = $1 AND indexname NOT IN (SELECT conname FROM pg_constraint) ORDER BY indexname", name); err != nil {
			return nil, err
		}
		cols := make([]string, len(t.Columns))
		for i, c := range t.Columns {
			cols[i] = "encode(convert_to(" + c.Name + "::text, 'UTF8'), 'hex')"
			if strings.HasPrefix(c.Type, "bytea") {
				cols[i] = "encode(" + c.Name + ", 'hex')"
			}
		}
		order := make([]string, len(cols))
		for i := range cols {
			order[i] = fmt.Sprint(i + 1)
		}
		r, err := db.QueryContext(ctx, "SELECT "+strings.Join(cols, ", ")+" FROM "+name+" ORDER BY "+strings.Join(order, ", ")) //nolint:gosec // generator, fixed table names
		if err != nil {
			return nil, err
		}
		for r.Next() {
			vals := make([]*string, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := r.Scan(ptrs...); err != nil {
				return nil, err
			}
			t.Rows = append(t.Rows, vals)
		}
		if err := r.Close(); err != nil {
			return nil, err
		}
	}
	return tables, nil
}

func queryStrings(ctx context.Context, db *sql.DB, q, arg string) ([]string, error) {
	rows, err := db.QueryContext(ctx, q, arg)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func fail(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

