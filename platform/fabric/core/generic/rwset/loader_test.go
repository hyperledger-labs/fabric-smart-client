/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rwset

import (
	stderrors "errors"
	"testing"

	"github.com/stretchr/testify/require"

	rwsetfake "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/rwset/fake"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	txmock "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

// TestGetRWSetFromETx_DoesNotValidate checks that the loader reads back a
// transaction this node stored, even when the channel no longer accepts its
// creator: validation applies to transactions received from peers, not to
// the node's own store.
func TestGetRWSetFromETx_DoesNotValidate(t *testing.T) {
	t.Parallel()
	cm := &txmock.ChannelMembership{}
	cm.IsValidReturns(stderrors.New("creator revoked"))
	vault := &txmock.Vault{}
	vault.NewRWSetReturns(&txmock.RWSet{}, nil)
	ch := &txmock.Channel{}
	ch.ChannelMembershipReturns(cm)
	ch.VaultReturns(vault)
	channels := &txmock.ChannelProvider{}
	channels.ChannelReturns(ch, nil)

	tm := transaction.NewManager()
	tm.AddTransactionFactory(driver.EndorserTransaction, transaction.NewEndorserTransactionFactory("network", channels, &txmock.SignerService{}))
	stored, err := tm.NewTransaction(t.Context(), driver.EndorserTransaction, []byte("creator"), []byte("nonce"), "tx1", "channel", nil)
	require.NoError(t, err)
	raw, err := stored.Bytes()
	require.NoError(t, err)

	loader := NewLoader("network", "channel", nil,
		&rwsetfake.EndorserTransactionService{ExistsValue: true, Transaction: raw}, tm, nil)
	rws, tx, err := loader.GetRWSetFromETx(t.Context(), "tx1")
	require.NoError(t, err)
	require.NotNil(t, rws)
	require.Equal(t, "tx1", tx.ID())
	require.Zero(t, cm.IsValidCallCount())
}
