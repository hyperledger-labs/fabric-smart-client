/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package otx

import (
	"crypto/rand"
	"github.com/hyperledger-labs/orion-server/pkg/types"
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/orion"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Transaction struct {
	SP        view2.ServiceProvider
	Network   string
	Namespace string

	Creator view.Identity
	Nonce   []byte
	TxID    string

	ONS    *orion.NetworkService
	DataTx *orion.Transaction
}

func NewTransaction(sp view2.ServiceProvider, id string) (*Transaction, error) {
	nonce, err := getRandomNonce()
	if err != nil {
		return nil, err
	}
	t := &Transaction{
		SP:      sp,
		Creator: view.Identity(id),
		Nonce:   nonce,
	}
	if _, err := t.getDataTx(); err != nil {
		return nil, errors.WithMessage(err, "failed to get data tx")
	}
	return t, nil
}

func (t *Transaction) SetNamespace(ns string) {
	t.Namespace = ns
}

func (t *Transaction) ID() string {
	return t.TxID
}

func (t *Transaction) Get(key string) ([]byte, *types.Metadata, error) {
	s, err := t.getDataTx()
	if err != nil {
		return nil, nil, err
	}
	return s.Get(t.Namespace, key)
}

func (t *Transaction) Put(key string, bytes []byte, a *types.AccessControl) error {
	s, err := t.getDataTx()
	if err != nil {
		return err
	}
	return s.Put(t.Namespace, key, bytes, a)
}

func (t *Transaction) getDataTx() (*orion.Transaction, error) {
	if t.DataTx == nil {
		var err error
		// set tx id
		ons := t.GetONS()
		txID := &orion.TxID{
			Nonce:   t.Nonce,
			Creator: []byte(t.Creator),
		}
		id := ons.TransactionManager().ComputeTxID(txID)
		t.DataTx, err = ons.TransactionManager().NewTransaction(id, string(t.Creator))
		if err != nil {
			return nil, errors.WithMessagef(err, "failed getting data tx for [%s]", id)
		}
		t.TxID = id
	}
	return t.DataTx, nil
}

func (t *Transaction) GetONS() *orion.NetworkService {
	if t.ONS == nil {
		t.ONS = orion.GetOrionNetworkService(t.SP, t.Network)
	}
	return t.ONS
}

func getRandomNonce() ([]byte, error) {
	key := make([]byte, 24)

	_, err := rand.Read(key)
	if err != nil {
		return nil, errors.Wrap(err, "error getting random bytes")
	}
	return key, nil
}
