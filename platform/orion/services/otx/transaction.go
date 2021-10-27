/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package otx

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"

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

	ID      string
	Creator view.Identity
	Nonce   []byte
	TxID    string

	ONS     *orion.NetworkService
	Session *orion.Session
	DataTx  *orion.Transaction
}

func NewTransaction(sp view2.ServiceProvider, id string) (*Transaction, error) {
	nonce, err := getRandomNonce()
	if err != nil {
		return nil, err
	}

	return &Transaction{
		SP:      sp,
		ID:      id,
		Creator: view.Identity(id),
		Nonce:   nonce,
		TxID:    ComputeTxID(nonce, []byte(id)),
	}, nil
}

func (t *Transaction) SetNamespace(ns string) {
	t.Namespace = ns
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
	if t.Session == nil {
		var err error
		t.Session, err = t.getOns().SessionManager().NewSession(t.ID)
		if err != nil {
			return nil, errors.WithMessagef(err, "failed getting session for [%s]", t.ID)
		}
		t.DataTx, err = t.Session.NewTransaction(t.TxID)
		if err != nil {
			return nil, errors.WithMessagef(err, "failed getting data tx for [%s]", t.ID)
		}
	}
	return t.DataTx, nil
}

func (t *Transaction) getOns() *orion.NetworkService {
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

func ComputeTxID(nonce, creator []byte) string {
	// TODO: Get the Hash function to be used from
	// channel configuration
	hasher := sha256.New()
	hasher.Write(nonce)
	hasher.Write(creator)
	return hex.EncodeToString(hasher.Sum(nil))
}
