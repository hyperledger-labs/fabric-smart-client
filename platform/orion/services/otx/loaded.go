/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package otx

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

type LoadedTransaction struct {
	SP        view2.ServiceProvider
	Network   string
	Namespace string

	Creator view.Identity
	Nonce   []byte
	TxID    string
	Env     []byte

	ONS          *orion.NetworkService
	LoadedDataTx *orion.LoadedTransaction
}

func NewLoadedTransaction(sp view2.ServiceProvider, id, network, namespace string, env []byte) (*LoadedTransaction, error) {
	nonce, err := getRandomNonce()
	if err != nil {
		return nil, err
	}
	lt := &LoadedTransaction{
		SP:        sp,
		Creator:   view.Identity(id),
		Network:   network,
		Namespace: namespace,
		Nonce:     nonce,
		Env:       env,
	}
	if _, err := lt.getLoadedDataTx(); err != nil {
		return nil, errors.WithMessage(err, "failed to get loaded data tx")
	}
	return lt, nil
}

func (lt *LoadedTransaction) ID() string {
	t, err := lt.getLoadedDataTx()
	if err != nil {
		return ""
	}
	return t.ID()
}

func (lt *LoadedTransaction) Commit() error {
	t, err := lt.getLoadedDataTx()
	if err != nil {
		return err
	}
	return t.Commit()
}

func (lt *LoadedTransaction) CoSignAndClose() ([]byte, error) {
	t, err := lt.getLoadedDataTx()
	if err != nil {
		return nil, err
	}
	return t.CoSignAndClose()
}

func (lt *LoadedTransaction) Reads() []*orion.DataRead {
	t, err := lt.getLoadedDataTx()
	if err != nil {
		return nil
	}
	return t.Reads()[lt.Namespace]
}

func (lt *LoadedTransaction) Writes() []*orion.DataWrite {
	t, err := lt.getLoadedDataTx()
	if err != nil {
		return nil
	}
	return t.Writes()[lt.Namespace]
}

func (lt *LoadedTransaction) MustSignUsers() []string {
	t, err := lt.getLoadedDataTx()
	if err != nil {
		return nil
	}
	return t.MustSignUsers()
}

func (lt *LoadedTransaction) SignedUsers() []string {
	t, err := lt.getLoadedDataTx()
	if err != nil {
		return nil
	}
	return t.SignedUsers()
}

func (lt *LoadedTransaction) getLoadedDataTx() (*orion.LoadedTransaction, error) {
	if lt.LoadedDataTx == nil {
		var err error
		// set tx id
		ons, err := lt.GetONS()
		if err != nil {
			return nil, err
		}
		lt.LoadedDataTx, err = ons.TransactionManager().NewLoadedTransaction(lt.Env, string(lt.Creator))
		if err != nil {
			return nil, errors.WithMessagef(err, "failed getting data tx for [%s]", lt.Creator)
		}
	}
	return lt.LoadedDataTx, nil
}

func (lt *LoadedTransaction) GetONS() (*orion.NetworkService, error) {
	if lt.ONS == nil {
		ons, err := orion.GetOrionNetworkService(lt.SP, lt.Network)
		if err != nil {
			return nil, err
		}
		lt.ONS = ons
	}
	return lt.ONS, nil
}
