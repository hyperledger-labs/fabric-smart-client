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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/multiplexed"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/storage/endorsetx"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
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

	stored, err := ets.LoadTransaction(ctx, txID)
	require.NoError(t, err)
	require.Equal(t, first, stored, "the first record is kept")
}
