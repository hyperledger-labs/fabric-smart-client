/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// Endorse builds a transaction on Network, writes one state to Namespace, and
// commits it with the endorsement of this node and of the FSC node Endorser. The
// Endorser receives the transaction as bytes, so the spec covers which local
// network the receiver decodes it on.
type Endorse struct {
	Network   string
	Namespace string
	Endorser  string
}

// Call returns the txID of the committed transaction.
func (e *Endorse) Call(viewCtx view.Context) (any, error) {
	_, etx, err := endorser.NewTransactionWithSigner(viewCtx, e.Network, "", nil)
	if err != nil {
		return nil, errors.WithMessagef(err, "network [%s]: creating a transaction", e.Network)
	}
	tx, err := state.Wrap(etx)
	if err != nil {
		return nil, err
	}
	tx.SetNamespace(e.Namespace)
	if err := tx.AddOutput(&struct{ Value string }{Value: "endorse"}); err != nil {
		return nil, err
	}
	me := tx.FabricNetworkService().IdentityProvider().DefaultIdentity()
	idProvider, err := id.GetProvider(viewCtx)
	if err != nil {
		return nil, err
	}
	other := idProvider.Identity(e.Endorser)
	if _, err := viewCtx.RunView(state.NewCollectEndorsementsView(tx, me, other)); err != nil {
		return nil, errors.WithMessagef(err, "network [%s]: collecting endorsements", e.Network)
	}
	if _, err := viewCtx.RunView(state.NewOrderingAndFinalityWithTimeoutView(tx, time.Minute)); err != nil {
		return nil, errors.WithMessagef(err, "network [%s]: committing a transaction", e.Network)
	}
	return tx.ID(), nil
}

type EndorseFactory struct{}

func (*EndorseFactory) NewView(in []byte) (view.View, error) {
	e := &Endorse{}
	if err := json.Unmarshal(in, e); err != nil {
		return nil, errors.Wrap(err, "failed unmarshalling input")
	}
	return e, nil
}

// Approve is the responder of Endorse: it receives the transaction, endorses it
// with this node's identity on the network the transaction was decoded on, and
// waits until it is final there.
type Approve struct{}

func (*Approve) Call(viewCtx view.Context) (any, error) {
	tx, err := state.ReceiveTransaction(viewCtx)
	if err != nil {
		return nil, errors.WithMessage(err, "receiving the transaction")
	}
	if _, err := viewCtx.RunView(state.NewEndorseView(tx)); err != nil {
		return nil, errors.WithMessagef(err, "network [%s]: endorsing", tx.Network())
	}
	if _, err := viewCtx.RunView(state.NewFinalityWithTimeoutView(tx, time.Minute)); err != nil {
		return nil, errors.WithMessagef(err, "network [%s]: waiting for finality", tx.Network())
	}
	return nil, nil
}

// Status returns the validation code of TxID in the vault of the default channel
// of Network, as that vault reports it now.
type Status struct {
	Network string
	TxID    string
}

func (s *Status) Call(viewCtx view.Context) (any, error) {
	_, ch, err := fabric.GetChannel(viewCtx, s.Network, "")
	if err != nil {
		return nil, errors.WithMessagef(err, "network [%s]", s.Network)
	}
	code, _, err := ch.Vault().Status(viewCtx.Context(), s.TxID)
	if err != nil {
		return nil, errors.WithMessagef(err, "network [%s]: reading the status of tx [%s]", s.Network, s.TxID)
	}
	return code, nil
}

type StatusFactory struct{}

func (*StatusFactory) NewView(in []byte) (view.View, error) {
	s := &Status{}
	if err := json.Unmarshal(in, s); err != nil {
		return nil, errors.Wrap(err, "failed unmarshalling input")
	}
	return s, nil
}
