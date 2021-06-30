/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/session"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var logger = flogging.MustGetLogger("fabric-sdk.services.endorser")

type Builder struct {
	sp view2.ServiceProvider
}

func NewBuilder(context view.Context) *Builder {
	if context == nil {
		panic("context must be set")
	}
	return &Builder{sp: context}
}

func NewBuilderWithServiceProvider(sp view2.ServiceProvider) *Builder {
	if sp == nil {
		panic("service provider must be set")
	}
	return &Builder{sp: sp}
}

func (t *Builder) NewTransaction() (*Transaction, error) {
	return t.NewTransactionForChannel("")
}

func (t *Builder) NewTransactionForChannel(channel string) (*Transaction, error) {
	return t.newTransaction(nil, "", channel, nil, nil, false)
}

func (t *Builder) NewTransactionFromBytes(bytes []byte) (*Transaction, error) {
	tx, err := t.newTransaction(nil, "", "", nil, bytes, false)
	if err != nil {
		return nil, err
	}

	return tx, nil
}

func (t *Builder) NewTransactionFromEnvelopeBytes(bytes []byte) (*Transaction, error) {
	tx, err := t.newTransaction(nil, "", "", nil, bytes, true)
	if err != nil {
		return nil, err
	}

	return tx, nil
}

func (t *Builder) NewTransactionWithIdentity(id view.Identity) (*Transaction, error) {
	logger.Debugf("NewTransactionWithIdentity with identity %s\n", id.UniqueID())

	tx, err := t.newTransaction(id, "", "", nil, nil, false)
	if err != nil {
		return nil, err
	}

	return tx, nil
}

func (t *Builder) newTransaction(creator []byte, network, channel string, nonce, raw []byte, envelope bool) (*Transaction, error) {
	logger.Debugf("NewTransaction [%s,%s,%s]", view.Identity(creator).UniqueID(), channel, hash.Hashable(raw).String())
	defer logger.Debugf("NewTransaction...done.")

	fNetwork := fabric.GetFabricNetworkService(t.sp, network)
	if len(creator) == 0 {
		creator = fNetwork.IdentityProvider().DefaultIdentity()
	}
	fabricTransaction, err := fNetwork.TransactionManager().NewTransaction(
		fabric.WithCreator(creator),
		fabric.WithNonce(nonce),
		fabric.WithChannel(channel),
	)
	if err != nil {
		return nil, err
	}

	tx := &Transaction{
		ServiceProvider: t.sp,
		Transaction:     fabricTransaction,
	}

	if len(raw) != 0 {
		if envelope {
			err = tx.SetFromEnvelopeBytes(raw)
		} else {
			err = tx.SetFromBytes(raw)
		}
		if err != nil {
			return nil, err
		}
	}
	return tx, nil
}

func NewTransaction(context view.Context) (*Builder, *Transaction, error) {
	txBuilder := NewBuilder(context)
	tx, err := txBuilder.NewTransaction()
	if err != nil {
		return nil, nil, err
	}
	return txBuilder, tx, nil
}

func NewTransactionWith(sp view2.ServiceProvider, network, channel string, id view.Identity) (*Builder, *Transaction, error) {
	txBuilder := NewBuilderWithServiceProvider(sp)
	tx, err := txBuilder.newTransaction(id, network, channel, nil, nil, false)
	if err != nil {
		return nil, nil, err
	}
	return txBuilder, tx, nil
}

func NewTransactionFromBytes(context view.Context, bytes []byte) (*Builder, *Transaction, error) {
	txBuilder := NewBuilder(context)
	tx, err := txBuilder.NewTransactionFromBytes(bytes)
	if err != nil {
		return nil, nil, err
	}
	return txBuilder, tx, nil
}

func NewTransactionFromEnvelopeBytes(sp view2.ServiceProvider, bytes []byte) (*Builder, *Transaction, error) {
	txBuilder := NewBuilderWithServiceProvider(sp)
	tx, err := txBuilder.NewTransactionFromEnvelopeBytes(bytes)
	if err != nil {
		return nil, nil, err
	}
	return txBuilder, tx, nil
}

func ReceiveTransaction(context view.Context) (*Transaction, error) {
	_, tx, err := NewTransactionFromBytes(context, session.ReadFirstMessageOrPanic(context))
	return tx, err
}
