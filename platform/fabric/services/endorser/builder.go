/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var logger = logging.MustGetLogger()

type Builder struct {
	sp services.Provider
}

// NewBuilder returns a Builder that creates transactions using the given service provider.
// It returns an error if sp is nil.
func NewBuilder(sp services.Provider) (*Builder, error) {
	if sp == nil {
		return nil, errors.New("service provider must be set")
	}
	return &Builder{sp: sp}, nil
}

// NewTransaction creates a transaction on the default network from opts.
// fabric.WithTxID sets its ID. Receiving endorsers on Fabric refuse a txID
// that is not derived from the nonce and creator (ValidateReceived). Fabric-x
// does not check that derivation, and tests use the option to choose an ID.
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
		fabricOptions.TxID,
	)
}

// NewTransactionFromBytes decodes a transaction from its serialized form on the
// local network whose channel matches the ChannelKey in bytes; see routeReceived.
// Bytes without a key are decoded on the default network. It does not validate
// data received from a peer; use ValidateReceived.
func (t *Builder) NewTransactionFromBytes(bytes []byte) (*Transaction, error) {
	return t.receivedTransactionFromBytes("", bytes)
}

// receivedTransactionFromBytes is NewTransactionFromBytes for bytes received from
// sender, which names the sender in the warning for bytes without a key.
func (t *Builder) receivedTransactionFromBytes(sender string, bytes []byte) (*Transaction, error) {
	if t.sp == nil {
		return nil, errors.New("service provider must be set")
	}
	network, key, err := routeReceived(t.sp, sender, bytes)
	if err != nil {
		return nil, err
	}
	tx, err := t.buildTransaction(context.Background(), nil, network, "", nil, bytes, false)
	if err != nil {
		return nil, err
	}
	// The key picks the network; the transaction must also be on the key's channel.
	if key != nil && tx.Channel() != key.Channel {
		tx.Close()
		return nil, errors.Errorf("transaction channel [%s] does not match its channel key [%s]", tx.Channel(), key.Channel)
	}
	return tx, nil
}

// NewTransactionFromEnvelopeBytes decodes an endorser transaction envelope on the
// default network. An envelope carries no channel key; use
// NewTransactionFromEnvelopeBytesOn for an envelope of another network.
func (t *Builder) NewTransactionFromEnvelopeBytes(ctx context.Context, bytes []byte) (*Transaction, error) {
	return t.buildTransaction(ctx, nil, "", "", nil, bytes, true)
}

func (t *Builder) NewTransactionWithIdentity(id view.Identity) (*Transaction, error) {
	logger.Debugf("NewTransactionWithIdentity with identity %s\n", id.UniqueID())

	tx, err := t.buildTransaction(context.Background(), id, "", "", nil, nil, false)
	if err != nil {
		return nil, err
	}

	return tx, nil
}

func (t *Builder) buildTransaction(ctx context.Context, creator []byte, network, channel string, nonce, raw []byte, envelope bool) (*Transaction, error) {
	return t.newTransactionWithType(ctx, creator, network, channel, nonce, raw, envelope, nil, nil, "")
}

func (t *Builder) newTransactionWithType(ctx context.Context, creator []byte, network, channel string, nonce, raw []byte, envelope bool, rawRequest []byte, tType *fabric.TransactionType, txid string) (*Transaction, error) {
	logger.Debugf("NewTransaction [%s,%s,%s]", view.Identity(creator).UniqueID(), channel, logging.SHA256Base64(raw))
	defer logger.Debugf("NewTransaction...done.")

	if t.sp == nil {
		return nil, errors.New("service provider must be set")
	}
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
	if len(txid) != 0 {
		options = append(options, fabric.WithTxID(txid))
	}

	var fabricTransaction *fabric.Transaction
	switch {
	case len(raw) == 0:
		fabricTransaction, err = fNetwork.TransactionManager().NewTransaction(options...)
	case envelope:
		fabricTransaction, err = fNetwork.TransactionManager().NewTransactionFromEnvelopeBytes(raw, options...)
	default:
		fabricTransaction, err = fNetwork.TransactionManager().NewTransactionFromBytes(raw, options...)
	}

	if err != nil {
		return nil, err
	}
	return &Transaction{
		Provider:    t.sp,
		Transaction: fabricTransaction,
	}, nil
}

func NewTransaction(viewCtx view.Context, opts ...fabric.TransactionOption) (*Builder, *Transaction, error) {
	txBuilder, err := NewBuilder(viewCtx)
	if err != nil {
		return nil, nil, err
	}
	tx, err := txBuilder.NewTransaction(viewCtx.Context(), opts...)
	if err != nil {
		return nil, nil, err
	}
	viewCtx.OnError(tx.Close)
	return txBuilder, tx, nil
}

// NewTransactionFromBytes decodes a transaction received from the caller of the
// session of viewCtx; see NewTransactionFromPeer.
func NewTransactionFromBytes(viewCtx view.Context, bytes []byte) (*Builder, *Transaction, error) {
	return NewTransactionFromPeer(viewCtx, sessionCaller(viewCtx), bytes)
}

// NewTransactionFromPeer decodes a transaction received from peer on the local
// network its ChannelKey names (see Builder.NewTransactionFromBytes) and validates
// it with ValidateReceived against that network before returning it. A failure
// that wraps ErrInvalidTransaction is reported to security.Rejections under peer.
func NewTransactionFromPeer(viewCtx view.Context, peer view.Identity, bytes []byte) (*Builder, *Transaction, error) {
	txBuilder, err := NewBuilder(viewCtx)
	if err != nil {
		return nil, nil, err
	}
	tx, err := txBuilder.receivedTransactionFromBytes(peer.String(), bytes)
	if err != nil {
		return nil, nil, err
	}
	if err := checkReceived(viewCtx, peer, tx.Transaction); err != nil {
		tx.Close()
		return nil, nil, err
	}
	viewCtx.OnError(tx.Close)
	return txBuilder, tx, nil
}

func NewTransactionWithSigner(viewCtx view.Context, network, channel string, id view.Identity) (*Builder, *Transaction, error) {
	txBuilder, err := NewBuilder(viewCtx)
	if err != nil {
		return nil, nil, err
	}
	tx, err := txBuilder.buildTransaction(viewCtx.Context(), id, network, channel, nil, nil, false)
	if err != nil {
		return nil, nil, err
	}
	viewCtx.OnError(tx.Close)
	return txBuilder, tx, nil
}

func NewTransactionWith(ctx context.Context, sp services.Provider, network, channel string, id view.Identity) (*Builder, *Transaction, error) {
	txBuilder, err := NewBuilder(sp)
	if err != nil {
		return nil, nil, err
	}
	tx, err := txBuilder.buildTransaction(ctx, id, network, channel, nil, nil, false)
	if err != nil {
		return nil, nil, err
	}
	return txBuilder, tx, nil
}

// NewTransactionFromEnvelopeBytes decodes an endorser transaction envelope on the
// default network; see NewTransactionFromEnvelopeBytesOn.
func NewTransactionFromEnvelopeBytes(ctx context.Context, sp services.Provider, bytes []byte) (*Builder, *Transaction, error) {
	return NewTransactionFromEnvelopeBytesOn(ctx, sp, "", bytes)
}

// NewTransactionFromEnvelopeBytesOn decodes an endorser transaction envelope on the
// local network named network, the default network if it is empty. An envelope
// carries no channel key, so the caller names the network it belongs to.
func NewTransactionFromEnvelopeBytesOn(ctx context.Context, sp services.Provider, network string, bytes []byte) (*Builder, *Transaction, error) {
	txBuilder, err := NewBuilder(sp)
	if err != nil {
		return nil, nil, err
	}
	tx, err := txBuilder.buildTransaction(ctx, nil, network, "", nil, bytes, true)
	if err != nil {
		return nil, nil, err
	}
	return txBuilder, tx, nil
}
