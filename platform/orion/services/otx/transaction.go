/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package otx

import (
	"github.com/hyperledger-labs/orion-server/pkg/types"
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/orion"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
)

type transaction struct {
	sp      view.ServiceProvider
	network string
	id      string
	ns      string

	ons     *orion.NetworkService
	session *orion.Session
	dataTx  *orion.Transaction
}

func (t *transaction) SetNamespace(ns string) {
	t.ns = ns
}

func (t *transaction) Get(key string) ([]byte, *types.Metadata, error) {
	s, err := t.getDataTx()
	if err != nil {
		return nil, nil, err
	}
	return s.Get(t.ns, key)
}

func (t *transaction) Put(key string, bytes []byte, a *types.AccessControl) error {
	s, err := t.getDataTx()
	if err != nil {
		return err
	}
	return s.Put(t.ns, key, bytes, a)
}

func (t *transaction) getDataTx() (*orion.Transaction, error) {
	if t.session == nil {
		var err error
		t.session, err = t.getOns().SessionManager().NewSession(t.id)
		if err != nil {
			return nil, errors.WithMessagef(err, "failed getting session for [%s]", t.id)
		}
		t.dataTx, err = t.session.Transaction()
		if err != nil {
			return nil, errors.WithMessagef(err, "failed getting data tx for [%s]", t.id)
		}
	}
	return t.dataTx, nil
}

func (t *transaction) getOns() *orion.NetworkService {
	if t.ons == nil {
		t.ons = orion.GetOrionNetworkService(t.sp, t.network)
	}
	return t.ons
}

func NewTransaction(sp view.ServiceProvider, id string) (*transaction, error) {
	return &transaction{sp: sp, id: id}, nil
}
