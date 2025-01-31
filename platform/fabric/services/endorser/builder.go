/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"context"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/session"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var logger = logging.MustGetLogger("fabric-sdk.services.endorser")

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

func (t *Builder) NewTransaction(ctx context.Context, opts ...fabric.TransactionOption) (*Transaction, error) {
	fabricOptions, err := fabric.CompileTransactionOptions(opts...)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to compile options")
	}
	return t.newTransactionWithType(
		ctx,
		fabricOptions.Creator,
		"",
		fabricOptions.Channel,
		fabricOptions.Nonce,
		nil,
		false,
		fabricOptions.RawRequest,
		&fabricOptions.TransactionType,
	)
}

func (t *Builder) NewTransactionFromBytes(bytes []byte) (*Transaction, error) {
	tx, err := t.newTransaction(context.Background(), nil, "", "", nil, bytes, false)
	if err != nil {
		return nil, err
	}

	return tx, nil
}

func (t *Builder) NewTransactionFromEnvelopeBytes(ctx context.Context, bytes []byte) (*Transaction, error) {
	tx, err := t.newTransaction(ctx, nil, "", "", nil, bytes, true)
	if err != nil {
		return nil, err
	}

	return tx, nil
}

func (t *Builder) NewTransactionWithIdentity(id view.Identity) (*Transaction, error) {
	logger.Debugf("NewTransactionWithIdentity with identity %s\n", id.UniqueID())

	tx, err := t.newTransaction(context.Background(), id, "", "", nil, nil, false)
	if err != nil {
		return nil, err
	}

	return tx, nil
}

func (t *Builder) newTransaction(ctx context.Context, creator []byte, network, channel string, nonce, raw []byte, envelope bool) (*Transaction, error) {
	return t.newTransactionWithType(ctx, creator, network, channel, nonce, raw, envelope, nil, nil)
}

func (t *Builder) newTransactionWithType(ctx context.Context, creator []byte, network, channel string, nonce, raw []byte, envelope bool, rawRequest []byte, tType *fabric.TransactionType) (*Transaction, error) {
	logger.Debugf("NewTransaction [%s,%s,%s]", view.Identity(creator).UniqueID(), channel, hash.Hashable(raw).String())
	defer logger.Debugf("NewTransaction...done.")

	fNetwork, err := fabric.GetFabricNetworkService(t.sp, network)
	if err != nil {
		return nil, errors.WithMessagef(err, "fabric network service [%s] not found", network)
	}
	if len(creator) == 0 {
		creator = fNetwork.IdentityProvider().DefaultIdentity()
	}

	options := []fabric.TransactionOption{
		fabric.WithCreator(creator),
		fabric.WithNonce(nonce),
		fabric.WithChannel(channel),
		fabric.WithContext(ctx),
	}
	if tType != nil {
		options = append(options, fabric.WithTransactionType(*tType))
	}
	if rawRequest != nil {
		options = append(options, fabric.WithRawRequest(rawRequest))
	}

	var fabricTransaction *fabric.Transaction
	if len(raw) == 0 {
		fabricTransaction, err = fNetwork.TransactionManager().NewTransaction(options...)
	} else if envelope {
		fabricTransaction, err = fNetwork.TransactionManager().NewTransactionFromEnvelopeBytes(raw, options...)
	} else {
		fabricTransaction, err = fNetwork.TransactionManager().NewTransactionFromBytes(raw, options...)
	}

	if err != nil {
		return nil, err
	}
	return &Transaction{
		ServiceProvider: t.sp,
		Transaction:     fabricTransaction,
	}, nil
}

func NewTransaction(context view.Context, opts ...fabric.TransactionOption) (*Builder, *Transaction, error) {
	txBuilder := NewBuilder(context)
	tx, err := txBuilder.NewTransaction(context.Context(), opts...)
	if err != nil {
		return nil, nil, err
	}
	context.OnError(tx.Close)
	return txBuilder, tx, nil
}

func NewTransactionFromBytes(context view.Context, bytes []byte) (*Builder, *Transaction, error) {
	txBuilder := NewBuilder(context)
	tx, err := txBuilder.NewTransactionFromBytes(bytes)
	if err != nil {
		return nil, nil, err
	}
	context.OnError(tx.Close)
	return txBuilder, tx, nil
}

func NewTransactionWithSigner(context view.Context, network, channel string, id view.Identity) (*Builder, *Transaction, error) {
	txBuilder := NewBuilderWithServiceProvider(context)
	tx, err := txBuilder.newTransaction(context.Context(), id, network, channel, nil, nil, false)
	if err != nil {
		return nil, nil, err
	}
	context.OnError(tx.Close)
	return txBuilder, tx, nil
}

func NewTransactionWith(ctx context.Context, sp view2.ServiceProvider, network, channel string, id view.Identity) (*Builder, *Transaction, error) {
	txBuilder := NewBuilderWithServiceProvider(sp)
	tx, err := txBuilder.newTransaction(ctx, id, network, channel, nil, nil, false)
	if err != nil {
		return nil, nil, err
	}
	return txBuilder, tx, nil
}

func NewTransactionFromEnvelopeBytes(ctx context.Context, sp view2.ServiceProvider, bytes []byte) (*Builder, *Transaction, error) {
	txBuilder := NewBuilderWithServiceProvider(sp)
	tx, err := txBuilder.NewTransactionFromEnvelopeBytes(ctx, bytes)
	if err != nil {
		return nil, nil, err
	}
	return txBuilder, tx, nil
}

func ReceiveTransaction(context view.Context) (*Transaction, error) {
	_, tx, err := NewTransactionFromBytes(context, session.ReadFirstMessageOrPanic(context))
	return tx, err
}
