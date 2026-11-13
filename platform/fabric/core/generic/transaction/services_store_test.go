/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/protos"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/multiplexed"
	fsqlcommon "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/common"
	fsqlite "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/sqlite"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/storage/endorsetx"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// ecdsaSigner signs with a P-256 key. ECDSA signatures are randomized, so two
// signatures over the same message differ.
type ecdsaSigner struct{ key *ecdsa.PrivateKey }

func newECDSASigner(t *testing.T) *ecdsaSigner {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return &ecdsaSigner{key: key}
}

func (s *ecdsaSigner) Sign(message []byte) ([]byte, error) {
	digest := sha256.Sum256(message)
	return ecdsa.SignASN1(rand.Reader, s.key, digest[:])
}

// newRealEndorseTxStore returns an endorse-tx store backed by the real
// in-memory SQLite persistence.
func newRealEndorseTxStore(t *testing.T) driver.EndorseTxStore {
	t.Helper()
	cp := multiplexed.MockTypeConfig(mem.Persistence, struct{}{})
	d := multiplexed.NewDriver(cp, mem.NewNamedDriver(sqlite.NewDbProvider()))
	s, err := endorsetx.NewStore[driver.Key](cp, d, t.Name())
	require.NoError(t, err)
	return s
}

// TestEndorseTransactionServiceStoreTransaction stores Fabric transactions
// endorsed with a real ECDSA key under one txID. Two independent endorsements
// of the same signed proposal and results differ in their signatures and are
// the same record; a second endorser's response added to it is too. Different
// results, a different signed proposal or bytes that do not decode conflict.
func TestEndorseTransactionServiceStoreTransaction(t *testing.T) {
	t.Parallel()

	signer := newECDSASigner(t)
	alice, bob := view.Identity("alice"), view.Identity("bob")

	// proposal is a transaction holding only the signed proposal.
	proposal := func() []byte {
		f := newEndorsableTx(t)
		f.sigService.GetSignerReturns(signer, nil)
		require.NoError(t, f.tx.EndorseProposalWithIdentity(alice))
		raw, err := json.Marshal(f.tx)
		require.NoError(t, err)
		return raw
	}
	// endorse loads proposal into a fresh transaction, simulates results, has
	// ids endorse it and serializes it as the transaction manager does.
	endorse := func(proposal []byte, results string, ids ...view.Identity) []byte {
		f := newEndorsableTx(t)
		f.sigService.GetSignerReturns(signer, nil)
		f.rwset.BytesReturns([]byte(results), nil)
		require.NoError(t, f.tx.SetFromBytes(proposal))
		for _, id := range ids {
			require.NoError(t, f.tx.SetRWSet())
			require.NoError(t, f.tx.EndorseWithIdentity(id))
		}
		raw, err := f.tx.Bytes()
		require.NoError(t, err)
		out, err := json.Marshal(&transaction.SerializedTransaction{Type: driver.EndorserTransaction, Raw: raw})
		require.NoError(t, err)
		return out
	}

	p := proposal()
	first := endorse(p, "rws", alice)
	retry := endorse(p, "rws", alice)
	require.NotEqual(t, first, retry, "the two endorsements carry different signatures")

	ets := transaction.NewEndorseTransactionService(newRealEndorseTxStore(t), "network", "channel")
	ctx := t.Context()
	// The in-memory database is shared by every test in the process and by
	// repeated runs (-count), so the key is unique to this run.
	txID := "txid-" + rand.Text()
	require.NoError(t, ets.StoreTransaction(ctx, txID, first))
	require.NoError(t, ets.StoreTransaction(ctx, txID, retry), "a re-signed endorsement of the same results is not a conflict")
	require.NoError(t, ets.StoreTransaction(ctx, txID, endorse(p, "rws", alice, bob)), "a second endorser's response is not a conflict")

	// otherProposalBytes is first with the proposal bytes of another proposal and
	// the same proposal signature.
	otherProposalBytes := func() []byte {
		var other, outer transaction.SerializedTransaction
		require.NoError(t, json.Unmarshal(endorse(proposal(), "rws", alice), &other))
		require.NoError(t, json.Unmarshal(first, &outer))
		a, b := &transaction.Transaction{}, &transaction.Transaction{}
		require.NoError(t, json.Unmarshal(outer.Raw, a))
		require.NoError(t, json.Unmarshal(other.Raw, b))
		require.NotEqual(t, a.TSignedProposal.ProposalBytes, b.TSignedProposal.ProposalBytes)
		a.TSignedProposal.ProposalBytes = b.TSignedProposal.ProposalBytes
		var err error
		outer.Raw, err = json.Marshal(a)
		require.NoError(t, err)
		raw, err := json.Marshal(&outer)
		require.NoError(t, err)
		return raw
	}

	for name, raw := range map[string][]byte{
		"different results":         endorse(p, "other-rws", alice),
		"different signed proposal": endorse(proposal(), "rws", alice),
		"different proposal bytes":  otherProposalBytes(),
		"undecodable":               []byte("garbage"),
	} {
		require.ErrorIs(t, ets.StoreTransaction(ctx, txID, raw), dbdriver.ErrConflict, name)
	}

	// The memory driver opens a new store, which runs the contract step that stores envelopes.
	want, err := protos.EnvelopeFromJSON(first)
	require.NoError(t, err)
	stored, err := ets.LoadTransaction(ctx, txID)
	require.NoError(t, err)
	require.Equal(t, want, stored, "the first record is kept")
}

// TestSameEndorsementAcrossForms stores a Fabric endorsement as JSON and offers it again as an
// envelope. Re-signed endorsements of the same results are the same record in either form;
// different results conflict.
func TestSameEndorsementAcrossForms(t *testing.T) {
	t.Parallel()

	signer := newECDSASigner(t)
	alice := view.Identity("alice")
	f := newEndorsableTx(t)
	f.sigService.GetSignerReturns(signer, nil)
	require.NoError(t, f.tx.EndorseProposalWithIdentity(alice))
	p, err := json.Marshal(f.tx)
	require.NoError(t, err)
	endorse := func(results string) []byte {
		f := newEndorsableTx(t)
		f.sigService.GetSignerReturns(signer, nil)
		f.rwset.BytesReturns([]byte(results), nil)
		require.NoError(t, f.tx.SetFromBytes(p))
		require.NoError(t, f.tx.SetRWSet())
		require.NoError(t, f.tx.EndorseWithIdentity(alice))
		raw, err := (&transaction.WrappedTransaction{Transaction: f.tx, TransactionType: driver.EndorserTransaction}).Bytes()
		require.NoError(t, err)
		return raw
	}
	envelope := func(raw []byte) []byte {
		env, err := protos.EnvelopeFromJSON(raw)
		require.NoError(t, err)
		return env
	}

	ets := transaction.NewEndorseTransactionService(newRealEndorseTxStore(t), "network", "channel")
	ctx := t.Context()
	txID := "txid-" + rand.Text()
	require.NoError(t, ets.StoreTransaction(ctx, txID, endorse("rws")))
	require.NoError(t, ets.StoreTransaction(ctx, txID, envelope(endorse("rws"))), "an envelope of a re-signed endorsement is the same")
	require.ErrorIs(t, ets.StoreTransaction(ctx, txID, envelope(endorse("other-rws"))), dbdriver.ErrConflict)

	envID := "txid-" + rand.Text()
	require.NoError(t, ets.StoreTransaction(ctx, envID, envelope(endorse("rws"))))
	require.NoError(t, ets.StoreTransaction(ctx, envID, endorse("rws")), "JSON of a re-signed endorsement is the same as a stored envelope")
	require.ErrorIs(t, ets.StoreTransaction(ctx, envID, endorse("other-rws")), dbdriver.ErrConflict)
}

// TestStoreTransactionAcrossContractStep finds a Fabric endorsement stored as JSON, as an older
// release wrote it, in a SQLite store whose contract step is pending. The node refuses to open
// the store until the step has run, as node migrate --contract runs it. The row is then an
// envelope; the same endorsement written again, byte-equal or re-signed, is the same record,
// and different results conflict.
func TestStoreTransactionAcrossContractStep(t *testing.T) {
	t.Parallel()

	signer := newECDSASigner(t)
	alice := view.Identity("alice")
	f := newEndorsableTx(t)
	f.sigService.GetSignerReturns(signer, nil)
	require.NoError(t, f.tx.EndorseProposalWithIdentity(alice))
	p, err := json.Marshal(f.tx)
	require.NoError(t, err)
	endorse := func(results string) []byte {
		f := newEndorsableTx(t)
		f.sigService.GetSignerReturns(signer, nil)
		f.rwset.BytesReturns([]byte(results), nil)
		require.NoError(t, f.tx.SetFromBytes(p))
		require.NoError(t, f.tx.SetRWSet())
		require.NoError(t, f.tx.EndorseWithIdentity(alice))
		raw, err := (&transaction.WrappedTransaction{Transaction: f.tx, TransactionType: driver.EndorserTransaction}).Bytes()
		require.NoError(t, err)
		return raw
	}

	// The table exists before the store is versioned, so its contract step is pending.
	ds := "file:" + filepath.Join(t.TempDir(), "etx.sqlite")
	dbp := sqlite.NewDbProvider()
	rw, err := dbp.Get(sqlite.Opts{DataSource: ds})
	require.NoError(t, err)
	tables, err := fsqlcommon.GetTableNames("")
	require.NoError(t, err)
	raw, err := fsqlite.NewEndorseTxStore(rw, tables)
	require.NoError(t, err)
	require.NoError(t, raw.CreateSchema())
	cp := multiplexed.MockTypeConfig(fsqlite.Persistence, sqlite.Config{DataSource: ds})
	open := func() (driver.EndorserTransactionService, error) {
		s, err := endorsetx.NewStore[driver.Key](cp, multiplexed.NewDriver(cp, fsqlite.NewNamedDriver(cp, dbp)))
		if err != nil {
			return nil, err
		}
		return transaction.NewEndorseTransactionService(s, "network", "channel"), nil
	}

	ctx := t.Context()
	first := endorse("rws")
	key := driver.Key{Network: "network", Channel: "channel", TxID: "tx"}.UniqueKey()
	_, err = rw.WriteDB.ExecContext(ctx, "INSERT INTO "+raw.StoreName()+" (key, data) VALUES ($1, $2)", key, first)
	require.NoError(t, err)
	_, err = open()
	require.ErrorIs(t, err, migrate.ErrPending, "the store opens only after its contract step")

	require.NoError(t, migrate.Migrate(ctx, rw.WriteDB, migrate.SQLite, "", raw, migrate.WithContract()))
	ets, err := open()
	require.NoError(t, err)
	envelope, err := protos.EnvelopeFromJSON(first)
	require.NoError(t, err)
	stored, err := ets.LoadTransaction(ctx, "tx")
	require.NoError(t, err)
	require.Equal(t, envelope, stored, "the step rewrote the row as an envelope")

	require.NoError(t, ets.StoreTransaction(ctx, "tx", first), "the byte-equal endorsement is the same record")
	require.NoError(t, ets.StoreTransaction(ctx, "tx", endorse("rws")), "a re-signed endorsement is the same record")
	require.ErrorIs(t, ets.StoreTransaction(ctx, "tx", endorse("other-rws")), dbdriver.ErrConflict)
	stored, err = ets.LoadTransaction(ctx, "tx")
	require.NoError(t, err)
	require.Equal(t, envelope, stored, "the stored record is kept")
}
