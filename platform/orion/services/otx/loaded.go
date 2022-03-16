/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package otx

import (
	"github.com/golang/protobuf/proto"
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
	Env     proto.Message

	ONS          *orion.NetworkService
	LoadedDataTx *orion.LoadedTransaction
}

func NewLoadedTransaction(sp view2.ServiceProvider, id, network string, env proto.Message) (*LoadedTransaction, error) {
	nonce, err := getRandomNonce()
	if err != nil {
		return nil, err
	}
	lt := &LoadedTransaction{
		SP:      sp,
		Creator: view.Identity(id),
		Network: network,
		Nonce:   nonce,
		Env:     env,
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

func (lt *LoadedTransaction) CoSignAndClose() (proto.Message, error) {
	t, err := lt.getLoadedDataTx()
	if err != nil {
		return nil, err
	}
	return t.CoSignAndClose()
}

func (lt *LoadedTransaction) getLoadedDataTx() (*orion.LoadedTransaction, error) {
	if lt.LoadedDataTx == nil {
		var err error
		// set tx id
		ons := lt.GetONS()
		lt.LoadedDataTx, err = ons.TransactionManager().NewLoadedTransaction(lt.Env, string(lt.Creator))
		if err != nil {
			return nil, errors.WithMessagef(err, "failed getting data tx for [%s]", lt.Creator)
		}
	}
	return lt.LoadedDataTx, nil
}

func (lt *LoadedTransaction) GetONS() *orion.NetworkService {
	if lt.ONS == nil {
		lt.ONS = orion.GetOrionNetworkService(lt.SP, lt.Network)
	}
	return lt.ONS
}
