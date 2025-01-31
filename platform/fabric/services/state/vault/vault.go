/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	driver3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
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

type vaultStore interface {
	GetState(namespace driver.Namespace, key driver.PKey) (*driver.VersionedRead, error)
	GetStateRange(namespace driver.Namespace, startKey, endKey driver.PKey) (driver.TxStateIterator, error)
}
type localMembership interface {
	DefaultIdentity() view2.Identity
}

type vault struct {
	sp              driver3.ServiceProvider
	network         string
	channel         string
	vaultStore      vaultStore
	localMembership localMembership
}

func (f *vault) GetState(namespace driver.Namespace, id driver.PKey, state interface{}) error {
	value, err := f.vaultStore.GetState(namespace, id)
	if err != nil {
		return err
	}
	if value == nil || len(value.Raw) == 0 {
		return errors.Errorf("id [%s] not found", id)
	}

	if err := json.Unmarshal(value.Raw, state); err != nil {
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

	it, err := f.vaultStore.GetStateRange(ns, startKey, endKey)
	if err != nil {
		return nil, errors.Wrap(err, "failed getting state iterator")
	}
	return &ListStateQueryIteratorInterface{it: it}, nil
}

func (f *vault) GetStateCertification(namespace string, key string) ([]byte, error) {
	_, tx, err := endorser.NewTransactionWith(
		f.sp,
		f.network,
		f.channel,
		f.localMembership.DefaultIdentity(),
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

type service struct {
	sp   driver3.ServiceProvider
	fnsp driver2.FabricNetworkServiceProvider
}

func NewService(sp driver3.ServiceProvider, fnsp driver2.FabricNetworkServiceProvider) *service {
	return &service{sp: sp, fnsp: fnsp}
}

func (w *service) Vault(network string, channel string) (state.Vault, error) {
	fns, err := w.fnsp.FabricNetworkService(network)
	if err != nil {
		return nil, err
	}
	ch, err := fns.Channel(channel)
	if err != nil {
		return nil, err
	}
	return &vault{
		sp:              w.sp,
		network:         fns.Name(),
		channel:         ch.Name(),
		vaultStore:      ch.VaultStore(),
		localMembership: fns.LocalMembership(),
	}, nil
}
