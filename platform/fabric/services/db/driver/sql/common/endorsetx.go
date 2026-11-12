/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"context"
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/protos"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
)

// NewEndorseTxStore returns an endorsement-transaction store over table, reading
// through readDB and writing through writeDB. Its queries use $N placeholders,
// which both SQLite and Postgres accept.
func NewEndorseTxStore(writeDB common.WriteDB, readDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper, timeout common.QueryTimeout) *EndorseTxStore {
	return &EndorseTxStore{p: common.NewSimpleKeyDataStore(writeDB, readDB, table, errorWrapper, timeout)}
}

// envelopeStep is the contract step after which the store holds transactions as envelopes.
const envelopeStep = 2

// envelopePageSize is the number of rows the envelope step reads and rewrites at a time.
const envelopePageSize = 1000

// EndorseTxStore is a write-once store of endorsed transactions by key. It stores a transaction
// as a protobuf TransactionEnvelope (see [protos.EnvelopeFromJSON]) and opens only once its
// contract step 2 has run. Readers also accept the JSON transactions stored before the step
// and those that do not convert.
type EndorseTxStore struct {
	p *common.SimpleKeyDataStore
}

func (db *EndorseTxStore) GetEndorseTx(ctx context.Context, key string) ([]byte, error) {
	return db.p.GetData(ctx, key)
}

func (db *EndorseTxStore) ExistsEndorseTx(ctx context.Context, key string) (bool, error) {
	return db.p.ExistData(ctx, key)
}

// PutEndorseTx stores data under key. JSON that converts to an envelope (see [toEnvelope]) is
// stored as that envelope, the same bytes the contract step writes for it; any other data,
// including JSON that does not tell its platform, is stored as given. Writing the value already
// stored under key returns nil; writing different bytes returns an error wrapping
// [driver.ErrConflict] and keeps the stored value. A row an older release stored may hold the
// JSON of a transaction offered now as an envelope: callers that can receive a transaction
// twice compare the records on conflict.
func (db *EndorseTxStore) PutEndorseTx(ctx context.Context, key string, data []byte) error {
	if env, err := toEnvelope(data); err == nil {
		data = env
	}
	return db.p.PutDataStrict(ctx, key, data)
}

// errRoundTrip is returned by toEnvelope for JSON whose envelope reads back as a different
// transaction.
var errRoundTrip = errors.New("the envelope does not read back as the same transaction")

// toEnvelope returns the envelope of data if data is JSON (first byte '{') that
// protos.EnvelopeFromJSON converts and whose envelope protos.JSONFromEnvelope reads back as the
// same transaction (see protos.SameTransactionJSON), so that no transaction is replaced by an
// envelope its readers refuse or read differently, and an error otherwise. The conversion is
// deterministic: the same JSON always gives the same envelope.
//
// The store holds the transactions of every network of the node, so it cannot tell the
// platform of a transaction that is not wrapped in a Fabric SerializedTransaction and carries
// no channel key; such a transaction fails with protos.ErrNoChannelKey and stays JSON. A
// transaction of a platform whose format the binary does not register fails with
// protos.ErrNoFormat and stays JSON too.
func toEnvelope(data []byte) ([]byte, error) {
	if len(data) == 0 || data[0] != '{' {
		return nil, errors.New("not JSON")
	}
	env, err := protos.EnvelopeFromJSON(data)
	if err != nil {
		return nil, err
	}
	back, err := protos.JSONFromEnvelope(env)
	if err != nil {
		return nil, err
	}
	if !protos.SameTransactionJSON(data, back) {
		return nil, errRoundTrip
	}
	return env, nil
}

func (db *EndorseTxStore) CreateSchema() error {
	return db.p.CreateSchema()
}

// StoreName returns the table name; it identifies the store in the versions table.
func (db *EndorseTxStore) StoreName() string { return db.p.StoreName() }

// Steps returns the store's schema steps. Step 1 creates the table if it does not exist.
// Step 2, a required contract step, rewrites every JSON transaction that converts (see
// [toEnvelope]) as its envelope; binaries that read only JSON refuse the store afterwards, and
// this binary refuses it before.
func (db *EndorseTxStore) Steps() []migrate.Step {
	return append(db.p.Steps(), migrate.Step{
		Version:  envelopeStep,
		Name:     "transactions as envelopes",
		Go:       db.toEnvelopes,
		Contract: true,
		Required: true,
	})
}

// envelopeRow is a row of the endorse-tx table.
type envelopeRow struct {
	key  string
	data []byte
}

// toEnvelopes rewrites every JSON row of the table as its envelope, in pages of
// envelopePageSize rows in key order. Each page is read in full and its rows closed before its
// rows are updated, as a connection runs one statement at a time. Rows that are not JSON stay
// unchanged; JSON rows that do not convert stay unchanged too and are counted in one warning,
// which counts apart the transactions without a channel key and those of a platform without a
// registered format.
// A row is rewritten only if it still holds the bytes the step read, so the step never
// overwrites a value another node wrote after the read (on Postgres the step runs in READ
// COMMITTED without locking the table); such rows are counted separately.
func (db *EndorseTxStore) toEnvelopes(ctx context.Context, q migrate.Execer) error {
	table := db.p.StoreName()
	read, converted, changed, failed, keyless, noFormat := 0, 0, 0, 0, 0, 0
	// The first page has no lower bound, so that a row with the empty key is read too.
	query, args := "SELECT key, data FROM "+table+" ORDER BY key LIMIT $1", []any{envelopePageSize} //nolint:gosec // table name comes from configuration
	for {
		page, err := readPage(ctx, q, query, args...)
		if err != nil {
			return errors.Wrapf(err, "failed reading [%s]", table)
		}
		for _, r := range page {
			// toEnvelope checks this too; here it keeps non-JSON rows out of the failure count.
			if len(r.data) == 0 || r.data[0] != '{' {
				continue
			}
			env, err := toEnvelope(r.data)
			if errors.Is(err, protos.ErrNoChannelKey) {
				keyless++
				continue
			}
			if errors.Is(err, protos.ErrNoFormat) {
				noFormat++
				continue
			}
			if err != nil {
				failed++
				continue
			}
			res, err := q.ExecContext(ctx, "UPDATE "+table+" SET data = $1 WHERE key = $2 AND data = $3", env, r.key, r.data) //nolint:gosec // table name comes from configuration
			if err != nil {
				return errors.Wrapf(err, "failed rewriting key [%s] in [%s]", r.key, table)
			}
			n, err := res.RowsAffected()
			if err != nil {
				return errors.Wrapf(err, "failed rewriting key [%s] in [%s]", r.key, table)
			}
			if n == 0 {
				changed++
				continue
			}
			converted++
		}
		read += len(page)
		logger.Infof("store [%s]: read %d row(s), rewrote %d as envelopes, %d changed meanwhile and left as is, so far", table, read, converted, changed)
		if len(page) < envelopePageSize {
			break
		}
		query, args = "SELECT key, data FROM "+table+" WHERE key > $1 ORDER BY key LIMIT $2", []any{page[len(page)-1].key, envelopePageSize} //nolint:gosec // table name comes from configuration
	}
	if failed > 0 || keyless > 0 || noFormat > 0 {
		logger.Warnf("store [%s]: %d JSON transaction(s) stay JSON: %d with no channel key, %d with no format registered for their platform, %d that do not convert to an envelope", table, failed+keyless+noFormat, keyless, noFormat, failed)
	}
	logger.Infof("store [%s]: rewrote %d transaction(s) as envelopes; %d changed meanwhile and were left as is", table, converted, changed)
	return nil
}

// readPage runs query and returns all its rows, closed.
func readPage(ctx context.Context, q migrate.Execer, query string, args ...any) ([]envelopeRow, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var page []envelopeRow
	for rows.Next() {
		var r envelopeRow
		if err := rows.Scan(&r.key, &r.data); err != nil {
			return nil, err
		}
		page = append(page, r)
	}
	return page, rows.Err()
}
