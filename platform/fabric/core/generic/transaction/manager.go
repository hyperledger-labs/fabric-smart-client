/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package transaction

import (
	"crypto/rand"

	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/api"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Manager struct {
	sp  view.ServiceProvider
	fns api.FabricNetworkService
}

func NewManager(sp view.ServiceProvider, fns api.FabricNetworkService) *Manager {
	return &Manager{sp: sp, fns: fns}
}

func (m *Manager) ComputeTxID(id *api.TxID) string {
	return ComputeTxID(id)
}

func (m *Manager) NewEnvelope() api.Envelope {
	return NewEnvelope()
}

func (m *Manager) NewProposalResponseFromBytes(raw []byte) (api.ProposalResponse, error) {
	return NewProposalResponseFromBytes(raw)
}

func (m *Manager) NewTransaction(creator view2.Identity, nonce []byte, txid string, channel string) (api.Transaction, error) {
	ch, err := m.fns.Channel(channel)
	if err != nil {
		return nil, err
	}

	if len(nonce) == 0 {
		nonce, err = getRandomNonce()
		if err != nil {
			return nil, err
		}
	}
	if len(txid) == 0 {
		txid = protoutil.ComputeTxID(nonce, creator)
	}

	return &Transaction{
		sp:         m.sp,
		fns:        m.fns,
		vault:      ch,
		TCreator:   creator,
		TNonce:     nonce,
		TTxID:      txid,
		TChannel:   channel,
		TTransient: map[string][]byte{},
	}, nil
}

func (m *Manager) NewTransactionFromBytes(channel string, raw []byte) (api.Transaction, error) {
	ch, err := m.fns.Channel(channel)
	if err != nil {
		return nil, err
	}

	tx := &Transaction{
		sp:         m.sp,
		fns:        m.fns,
		vault:      ch,
		TChannel:   channel,
		TTransient: map[string][]byte{},
	}
	err = tx.SetFromBytes(raw)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func getRandomNonce() ([]byte, error) {
	key := make([]byte, 24)

	_, err := rand.Read(key)
	if err != nil {
		return nil, errors.Wrap(err, "error getting random bytes")
	}
	return key, nil
}
