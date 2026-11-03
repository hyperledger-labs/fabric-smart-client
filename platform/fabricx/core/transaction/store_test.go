/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/multiplexed"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/storage/endorsetx"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/transaction/mock"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
)

// ecdsaSerializableSigner signs with a P-256 key. ECDSA signatures are
// randomized, so two signatures over the same message differ.
type ecdsaSerializableSigner struct {
	key     *ecdsa.PrivateKey
	creator []byte
}

func (s *ecdsaSerializableSigner) Sign(message []byte) ([]byte, error) {
	digest := sha256.Sum256(message)
	return ecdsa.SignASN1(rand.Reader, s.key, digest[:])
}

func (s *ecdsaSerializableSigner) Serialize() ([]byte, error) { return s.creator, nil }

// TestEndorseTransactionServiceStoresFabricxTransactions stores Fabric-x
// transactions, which are serialized without a SerializedTransaction wrapper,
// endorsed with a real ECDSA key under one txID. Two endorsements of the same
// signed proposal and read-write set differ in their signatures and are the
// same record; a different read-write set conflicts.
func TestEndorseTransactionServiceStoresFabricxTransactions(t *testing.T) {
	t.Parallel()

	_, creator := mustSerializedIdentityWithRealCert(t, "Org1MSP")
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	signer := &ecdsaSerializableSigner{key: key, creator: creator}

	// The signed proposal is generated once and shared by every record.
	base := &Transaction{
		ctx: t.Context(), TTxID: "tx1", TNonce: []byte("nonce"), TCreator: creator,
		TChannel: "channel1", TChaincode: "cc", TFunction: "invoke", channel: endorsingChannel(),
	}
	require.NoError(t, base.EndorseWithSigner(creator, signer))
	endorse := func(value string) []byte {
		t.Helper()
		rws := &mock.RWSet{}
		rws.BytesReturns(mustRawTx(t, sampleTx("ns1", "key1", value)), nil)
		tx := &Transaction{
			ctx: base.ctx, TTxID: base.TTxID, TNonce: base.TNonce, TCreator: base.TCreator,
			TChannel: base.TChannel, TChaincode: base.TChaincode, TFunction: base.TFunction,
			TSignedProposal: base.TSignedProposal, signedProposal: base.signedProposal,
			channel: endorsingChannel(), rwSetHandle: rws,
		}
		require.NoError(t, tx.EndorseWithSigner(creator, signer))
		require.Len(t, tx.TProposalResponses, 1)
		raw, err := tx.Bytes()
		require.NoError(t, err)
		return raw
	}

	first, retry := endorse("v1"), endorse("v1")
	require.NotEqual(t, first, retry, "the two endorsements carry different signatures")

	cp := multiplexed.MockTypeConfig(mem.Persistence, struct{}{})
	store, err := endorsetx.NewStore[driver.Key](cp, multiplexed.NewDriver(cp, mem.NewNamedDriver(sqlite.NewDbProvider())), t.Name())
	require.NoError(t, err)
	ets := transaction.NewEndorseTransactionService(store, "network", "channel")
	ctx := t.Context()

	require.NoError(t, ets.StoreTransaction(ctx, "tx1", first))
	require.NoError(t, ets.StoreTransaction(ctx, "tx1", retry), "a re-signed endorsement of the same read-write set is not a conflict")
	require.ErrorIs(t, ets.StoreTransaction(ctx, "tx1", endorse("v2")), dbdriver.ErrConflict)

	stored, err := ets.LoadTransaction(ctx, "tx1")
	require.NoError(t, err)
	require.Equal(t, first, stored, "the first record is kept")
}
