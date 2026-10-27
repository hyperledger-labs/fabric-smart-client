/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package views holds the two-party views the faultrecovery suites run while they take
// parts of the network down: a transaction, and an echo that touches no ledger. The
// Fabric and Fabric-x suites share it.
package views

import (
	"encoding/json"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/session"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// Namespace is the namespace the suites deploy and write to. Its endorsement policy
// requires Org1, the initiator's organization.
const Namespace = "faultrecovery"

// FinalityTimeout bounds how long each party waits for a transaction to become final.
const FinalityTimeout = time.Minute

// Asset is the state each transaction creates. It is stored under the ID of the
// transaction that creates it, so a spec can look it up by transaction ID.
type Asset struct {
	ID    string
	Value int
}

// GetLinearID returns the key the Asset is stored under.
func (a *Asset) GetLinearID() (string, error) { return a.ID, nil }

// Submit is the input of EndorseView.
type Submit struct {
	// Value is stored in the created Asset.
	Value int
	// Responder is the FSC node that endorses the transaction after the initiator.
	Responder view.Identity
}

// Endorsed is what EndorseView returns.
type Endorsed struct {
	// Tx is the serialized transaction, the input of OrderView.
	Tx []byte
	// TxID is the transaction's ID, so a spec can wait for it before it is ordered.
	TxID string
}

// EndorseView creates an Asset and collects the initiator's and the responder's
// endorsements without ordering the transaction. It returns the transaction as
// Endorsed, so a spec can act on the network between endorsing and ordering it.
type EndorseView struct {
	Submit
}

func (v *EndorseView) Call(viewCtx view.Context) (any, error) {
	tx, err := state.NewTransaction(viewCtx)
	if err != nil {
		return nil, err
	}
	tx.SetNamespace(Namespace)
	if err := tx.AddOutput(&Asset{ID: tx.ID(), Value: v.Value}); err != nil {
		return nil, err
	}

	// The initiator signs first: it belongs to Org1, which the namespace policy requires.
	me := tx.FabricNetworkService().IdentityProvider().DefaultIdentity()
	if _, err := viewCtx.RunView(state.NewCollectEndorsementsView(tx, me, v.Responder)); err != nil {
		return nil, err
	}
	raw, err := tx.Bytes()
	if err != nil {
		return nil, err
	}
	return &Endorsed{Tx: raw, TxID: tx.ID()}, nil
}

type EndorseViewFactory struct{}

func (*EndorseViewFactory) NewView(in []byte) (view.View, error) {
	f := &EndorseView{}
	if err := json.Unmarshal(in, &f.Submit); err != nil {
		return nil, err
	}
	return f, nil
}

// Order is the input of OrderView.
type Order struct {
	// Tx is the serialized transaction of Endorsed.
	Tx []byte
}

// OrderView orders the transaction EndorseView returned and waits until it is final.
// It returns the transaction ID.
type OrderView struct {
	Order
}

func (v *OrderView) Call(viewCtx view.Context) (any, error) {
	tx, err := state.NewTransactionFromBytes(viewCtx, v.Tx)
	if err != nil {
		return nil, err
	}
	if _, err := viewCtx.RunView(state.NewOrderingAndFinalityWithTimeoutView(tx, FinalityTimeout)); err != nil {
		return nil, err
	}
	return tx.ID(), nil
}

type OrderViewFactory struct{}

func (*OrderViewFactory) NewView(in []byte) (view.View, error) {
	f := &OrderView{}
	if err := json.Unmarshal(in, &f.Order); err != nil {
		return nil, err
	}
	return f, nil
}

// ResponderView endorses the transaction EndorseView sends and waits until it is final.
type ResponderView struct{}

func (*ResponderView) Call(viewCtx view.Context) (any, error) {
	tx, err := state.ReceiveTransaction(viewCtx)
	if err != nil {
		return nil, err
	}
	if _, err := viewCtx.RunView(state.NewEndorseView(tx)); err != nil {
		return nil, err
	}
	return viewCtx.RunView(state.NewFinalityWithTimeoutView(tx, FinalityTimeout))
}

// Query is the input of QueryView.
type Query struct {
	TxID string
}

// QueryResult is what QueryView returns.
type QueryResult struct {
	// Status is the node's validation code for the transaction, a fabric driver.ValidationCode.
	Status int
	// Value is the value of the Asset the transaction created. It is set only when
	// Status is Valid.
	Value int
}

// QueryView reports what the node knows about a transaction: its status and, once
// it is valid, the value of the Asset it created. It reads the node's vault and
// does not wait for finality. On Fabric the vault is the node's own database; on
// Fabric-x it is the committer's query service, so the answer is the same on every node.
type QueryView struct {
	Query
}

func (v *QueryView) Call(viewCtx view.Context) (any, error) {
	_, ch, err := fabric.GetDefaultChannel(viewCtx)
	if err != nil {
		return nil, errors.WithMessage(err, "failed getting default channel")
	}
	res := &QueryResult{}
	if res.Status, _, err = ch.Vault().Status(viewCtx.Context(), v.TxID); err != nil {
		return nil, errors.WithMessagef(err, "failed getting status of [%s]", v.TxID)
	}
	if res.Status != fdriver.Valid {
		return res, nil
	}

	qe, err := ch.Vault().NewQueryExecutor(viewCtx.Context())
	if err != nil {
		return nil, errors.WithMessage(err, "failed getting query executor")
	}
	defer func() { _ = qe.Done() }()
	read, err := qe.GetState(viewCtx.Context(), Namespace, v.TxID)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed reading the asset of [%s]", v.TxID)
	}
	if read == nil || len(read.Raw) == 0 {
		return nil, errors.Errorf("tx [%s] is valid, but its asset is missing", v.TxID)
	}
	asset := &Asset{}
	if err := json.Unmarshal(read.Raw, asset); err != nil {
		return nil, errors.WithMessagef(err, "failed decoding the asset of [%s]", v.TxID)
	}
	res.Value = asset.Value
	return res, nil
}

type QueryViewFactory struct{}

func (*QueryViewFactory) NewView(in []byte) (view.View, error) {
	f := &QueryView{}
	if err := json.Unmarshal(in, &f.Query); err != nil {
		return nil, err
	}
	return f, nil
}

// echoTimeout bounds how long EchoView waits for the reply. It is well above the time a
// session takes to close after its counterparty dies, so a spec can tell the two apart.
const echoTimeout = time.Minute

// blockMessage asks EchoResponderView to hold the session open without replying.
const blockMessage = "block"

// Echo is the input of EchoView.
type Echo struct {
	// Responder is the FSC node that answers.
	Responder view.Identity
	// Block makes the responder receive the message and then wait, without replying,
	// until its view context ends, so a spec can kill the responder's node mid-view.
	Block bool
}

// EchoView sends one message to the responder and returns once the responder echoes it.
// It touches no ledger, so a spec can run a two-party view without Fabric in the way.
type EchoView struct {
	Echo
}

func (v *EchoView) Call(viewCtx view.Context) (any, error) {
	s, err := viewCtx.GetSession(viewCtx.Initiator(), v.Responder)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed getting session to [%s]", v.Responder)
	}
	msg := []byte("echo")
	if v.Block {
		msg = []byte(blockMessage)
	}
	if err := s.Send(viewCtx.Context(), msg); err != nil {
		return nil, errors.WithMessage(err, "failed sending")
	}
	reply, err := session.ReadMessageWithTimeout(s, echoTimeout)
	if err != nil {
		return nil, errors.WithMessage(err, "failed waiting for the echo")
	}
	return string(reply), nil
}

type EchoViewFactory struct{}

func (*EchoViewFactory) NewView(in []byte) (view.View, error) {
	f := &EchoView{}
	if err := json.Unmarshal(in, &f.Echo); err != nil {
		return nil, err
	}
	return f, nil
}

// EchoResponderView answers EchoView: it sends the message back, or, when asked to block,
// waits until its view context ends.
type EchoResponderView struct{}

func (*EchoResponderView) Call(viewCtx view.Context) (any, error) {
	s, msg, err := session.ReadFirstMessage(viewCtx)
	if err != nil {
		return nil, err
	}
	if string(msg) == blockMessage {
		<-viewCtx.Context().Done()
		return nil, viewCtx.Context().Err()
	}
	return nil, s.Send(viewCtx.Context(), msg)
}
