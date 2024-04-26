/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/pkg/errors"
)

type ListStateQueryIteratorInterface struct {
	it   fabric.ResultsIterator
	next *fabric.Read
}

func (l *ListStateQueryIteratorInterface) HasNext() bool {
	var err error
	l.next, err = l.it.Next()
	if err != nil || l.next == nil {
		return false
	}
	return true
}

func (l *ListStateQueryIteratorInterface) Close() error {
	l.it.Close()
	return nil
}

func (l *ListStateQueryIteratorInterface) Next(state interface{}) (string, error) {
	//log.Printf("It at %s\n", string(l.List[l.Index].Raw))
	return "", json.Unmarshal(l.next.Raw, state)
}

type NewQueryExecutorFunc func() (driver.QueryExecutor, error)

type vault struct {
	sp               view.ServiceProvider
	network          string
	channel          string
	NewQueryExecutor NewQueryExecutorFunc
}

func New(sp view.ServiceProvider, network, channel string, NewQueryExecutor func() (driver.QueryExecutor, error)) *vault {
	return &vault{
		sp:               sp,
		network:          network,
		channel:          channel,
		NewQueryExecutor: NewQueryExecutor,
	}
}

func (f *vault) GetState(namespace string, id string, state interface{}) error {
	q, err := f.NewQueryExecutor()
	if err != nil {
		return errors.Wrap(err, "failed getting query executor")
	}
	defer q.Done()

	raw, err := q.GetState(namespace, id)
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		return errors.Errorf("id [%s] not found", id)
	}

	err = json.Unmarshal(raw, state)
	if err != nil {
		return err
	}
	return nil
}

func (f *vault) GetStateByPartialCompositeID(ns string, prefix string, attrs []string) (state.QueryIteratorInterface, error) {
	startKey, err := state.CreateCompositeKey(prefix, attrs)
	if err != nil {
		return nil, err
	}
	endKey := startKey + string(state.MaxUnicodeRuneValue)

	q, err := f.NewQueryExecutor()
	if err != nil {
		return nil, errors.Wrap(err, "failed getting query executor")
	}
	defer q.Done()

	it, err := q.GetStateRangeScanIterator(ns, startKey, endKey)
	if err != nil {
		return nil, errors.Wrap(err, "failed getting state iterator")
	}
	return &ListStateQueryIteratorInterface{it: it}, nil
}

func (f *vault) GetStateCertification(namespace string, key string) ([]byte, error) {
	fns, err := fabric.GetFabricNetworkService(f.sp, f.network)
	if err != nil {
		return nil, err
	}
	_, tx, err := endorser.NewTransactionWith(
		f.sp,
		f.network,
		f.channel,
		fns.LocalMembership().DefaultIdentity(),
	)
	if err != nil {
		return nil, errors.Wrapf(err, "failed creating transaction [%s:%s]", namespace, key)
	}
	defer tx.Close()
	tx.SetProposal(namespace, "", "state_certification", namespace, key)
	rws, err := tx.RWSet()
	if err != nil {
		return nil, errors.Wrapf(err, "failed getting rws [%s:%s]", namespace, key)
	}
	v, err := rws.GetState(namespace, key)
	if err != nil {
		return nil, errors.Wrapf(err, "failed retrieving state [%s:%s]", namespace, key)
	}
	if err := rws.SetState(namespace, key, v); err != nil {
		return nil, errors.Wrapf(err, "failed setting state [%s:%s]", namespace, key)
	}
	err = tx.Endorse()
	if err != nil {
		return nil, errors.Wrapf(err, "failed endorsing transaction [%s:%s]", namespace, key)
	}
	raw, err := tx.Bytes()
	if err != nil {
		return nil, errors.Wrapf(err, "failed marshalling transaction [%s:%s]", namespace, key)
	}
	return raw, nil
}

type VaultFunc func(ctx view.ServiceProvider, id string) *fabric.Vault

type service struct {
	sp view.ServiceProvider
}

func NewService(sp view.ServiceProvider) *service {
	return &service{sp: sp}
}

func (w *service) Vault(network string, channel string) (state.Vault, error) {
	fns, err := fabric.GetFabricNetworkService(w.sp, network)
	if err != nil {
		return nil, err
	}
	ch, err := fns.Channel(channel)
	if err != nil {
		return nil, err
	}

	return New(
		w.sp,
		network,
		channel,
		ch.Vault().NewQueryExecutor,
	), nil
}
