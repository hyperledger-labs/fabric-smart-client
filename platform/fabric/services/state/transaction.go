/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Transaction struct {
	*endorser.Transaction
	*Namespace
}

func Wrap(tx *endorser.Transaction) (*Transaction, error) {
	if err := SetCertificationType(tx, ChaincodeCertification, nil); err != nil {
		return nil, errors.Wrap(err, "failed appending certification")
	}

	return &Transaction{
		Transaction: tx,
		Namespace:   NewNamespace(tx, false),
	}, nil
}

// NewTransaction returns a new instance of a state-based transaction that embeds a single namespace.
func NewTransaction(viewCtx view.Context) (*Transaction, error) {
	_, tx, err := endorser.NewTransaction(viewCtx)
	if err != nil {
		return nil, err
	}

	if err := SetCertificationType(tx, ChaincodeCertification, nil); err != nil {
		return nil, errors.Wrap(err, "failed appending certification")
	}

	return &Transaction{
		Transaction: tx,
		Namespace:   NewNamespace(tx, false),
	}, nil
}

// NewAnonymousTransaction returns a new instance of a state-based transaction that embeds a single namespace and is signed
// by an anonymous identity
func NewAnonymousTransaction(viewCtx view.Context) (*Transaction, error) {
	fns, err := fabric.GetDefaultFNS(viewCtx)
	if err != nil {
		return nil, err
	}
	anonIdentity, err := fns.LocalMembership().AnonymousIdentity()
	if err != nil {
		return nil, errors.WithMessagef(err, "failed getting anonymous identity")
	}
	_, tx, err := endorser.NewTransactionWithSigner(
		viewCtx,
		fns.Name(),
		fns.ConfigService().DefaultChannel(),
		anonIdentity,
	)
	if err != nil {
		return nil, err
	}

	if err := SetCertificationType(tx, ChaincodeCertification, nil); err != nil {
		return nil, errors.Wrap(err, "failed appending certification")
	}

	return &Transaction{
		Transaction: tx,
		Namespace:   NewNamespace(tx, false),
	}, nil
}

func NewTransactionFromBytes(viewCtx view.Context, raw []byte) (*Transaction, error) {
	_, tx, err := endorser.NewTransactionFromBytes(viewCtx, raw)
	if err != nil {
		return nil, err
	}

	return &Transaction{
		Transaction: tx,
		Namespace:   NewNamespace(tx, false),
	}, nil
}

type receiveTransactionView struct {
	party view.Identity
}

func NewReceiveTransactionView() *receiveTransactionView {
	return &receiveTransactionView{}
}

func NewReceiveTransactionFromView(party view.Identity) *receiveTransactionView {
	return &receiveTransactionView{party: party}
}

// ReceiveTransaction runs the receiveTransactionView that expects on the context's session
// a byte representation of a state transaction.
func ReceiveTransaction(viewCtx view.Context) (*Transaction, error) {
	txBoxed, err := viewCtx.RunView(NewReceiveTransactionView(), view.WithSameContext())
	if err != nil {
		return nil, err
	}

	cctx := txBoxed.(*Transaction)
	return cctx, nil
}

func ReceiveTransactionFrom(viewCtx view.Context, party view.Identity) (*Transaction, error) {
	txBoxed, err := viewCtx.RunView(NewReceiveTransactionFromView(party), view.WithSameContext())
	if err != nil {
		return nil, err
	}

	cctx := txBoxed.(*Transaction)
	return cctx, nil
}

func (f *receiveTransactionView) Call(viewCtx view.Context) (interface{}, error) {
	// Wait to receive a transaction back
	var ch <-chan *view.Message
	if f.party.IsNone() {
		ch = viewCtx.Session().Receive()
	} else {
		s, err := viewCtx.GetSession(viewCtx.Initiator(), f.party, f)
		if err != nil {
			return nil, err
		}
		ch = s.Receive()
	}

	timeout := time.NewTimer(time.Second * 10)
	defer timeout.Stop()

	select {
	case msg := <-ch:
		if msg.Status == view.ERROR {
			return nil, errors.New(string(msg.Payload))
		}
		tx, err := NewTransactionFromBytes(viewCtx, msg.Payload)
		if err != nil {
			return nil, err
		}
		return tx, nil
	case <-timeout.C:
		return nil, errors.New("timeout reached")
	}
}

type sendTransactionView struct {
	tx      *Transaction
	parties []view.Identity
}

func NewSendTransactionView(tx *Transaction, parties ...view.Identity) *sendTransactionView {
	return &sendTransactionView{tx: tx, parties: parties}
}

func (f *sendTransactionView) Call(viewCtx view.Context) (interface{}, error) {
	for _, party := range f.parties {
		logger.Debugf("Send transaction to [%s]", logging.SHA256Base64(party))

		if viewCtx.IsMe(party) {
			logger.Debugf("This is me %s, do not send.", logging.Base64(party))
			continue
		}

		txRaw, err := f.tx.Bytes()
		if err != nil {
			return nil, errors.Wrap(err, "failed marshalling transaction content")
		}

		session, err := viewCtx.GetSession(viewCtx.Initiator(), party)
		if err != nil {
			return nil, errors.Wrap(err, "failed getting session")
		}

		// Send transaction
		err = session.Send(txRaw)
		if err != nil {
			return nil, errors.Wrap(err, "failed sending transaction content")
		}
	}
	return nil, nil
}

type sendTransactionBackView struct {
	tx *Transaction
}

func NewSendTransactionBackView(tx *Transaction) *sendTransactionBackView {
	return &sendTransactionBackView{tx: tx}
}

func (f *sendTransactionBackView) Call(viewCtx view.Context) (interface{}, error) {
	txRaw, err := f.tx.Bytes()
	if err != nil {
		return nil, errors.Wrap(err, "failed marshalling transaction content")
	}

	session := viewCtx.Session()

	// Send transaction
	err = session.Send(txRaw)
	if err != nil {
		return nil, errors.Wrap(err, "failed sending transaction content")
	}

	return nil, nil
}

func SendAndReceiveTransaction(viewCtx view.Context, tx *Transaction, party view.Identity) (*Transaction, error) {
	_, err := viewCtx.RunView(NewSendTransactionView(tx, party), view.WithSameContext())
	if err != nil {
		return nil, err
	}

	return ReceiveTransactionFrom(viewCtx, party)
}

func SendBackAndReceiveTransaction(viewCtx view.Context, tx *Transaction) (*Transaction, error) {
	_, err := viewCtx.RunView(NewSendTransactionBackView(tx), view.WithSameContext())
	if err != nil {
		return nil, err
	}

	return ReceiveTransaction(viewCtx)
}
