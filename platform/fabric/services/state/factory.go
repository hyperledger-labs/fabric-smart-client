/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func NewViewFactory(
	fnsProvider *fabric.NetworkServiceProvider,
	endorseView *endorser.EndorseViewFactory,
	finalityView *endorser.FinalityViewFactory,
	collectEndorsementsView *endorser.CollectEndorsementsViewFactory,
	respondExchangeRecipientIdentitiesView *RespondExchangeRecipientIdentitiesViewFactory,
	exchangeRecipientIdentitiesView *ExchangeRecipientIdentitiesViewFactory,
	orderingAndFinalityView *endorser.OrderingAndFinalityViewFactory,
	receiveTransactionView *ReceiveTransactionViewFactory,
) *ViewFactory {
	return &ViewFactory{
		fnsProvider:                            fnsProvider,
		endorseView:                            endorseView,
		finalityView:                           finalityView,
		collectEndorsementsView:                collectEndorsementsView,
		respondExchangeRecipientIdentitiesView: respondExchangeRecipientIdentitiesView,
		exchangeRecipientIdentitiesView:        exchangeRecipientIdentitiesView,
		orderingAndFinalityView:                orderingAndFinalityView,
		receiveTransactionView:                 receiveTransactionView,
	}
}

type ViewFactory struct {
	fnsProvider                            *fabric.NetworkServiceProvider
	endorseView                            *endorser.EndorseViewFactory
	finalityView                           *endorser.FinalityViewFactory
	collectEndorsementsView                *endorser.CollectEndorsementsViewFactory
	respondExchangeRecipientIdentitiesView *RespondExchangeRecipientIdentitiesViewFactory
	exchangeRecipientIdentitiesView        *ExchangeRecipientIdentitiesViewFactory
	orderingAndFinalityView                *endorser.OrderingAndFinalityViewFactory
	receiveTransactionView                 *ReceiveTransactionViewFactory
}

func (f *ViewFactory) NewTransaction(context view.Context) (*Transaction, error) {
	return NewTransactionWithFNSP(f.fnsProvider, context)
}

func (f *ViewFactory) NewEndorseView(tx *Transaction, ids ...view.Identity) view.View {
	return f.endorseView.New(tx.tx, ids...)
}

// NewFinalityView returns a new instance of the finality view that waits for the finality of the passed transaction.
func (f *ViewFactory) NewFinalityView(tx *Transaction) view.View {
	return f.finalityView.New(tx.tx)
}

// NewFinalityWithTimeoutView runs the finality view for the passed transaction and timeout
func (f *ViewFactory) NewFinalityWithTimeoutView(tx *Transaction, timeout time.Duration) view.View {
	return f.finalityView.NewWithTimeout(tx.tx, timeout)
}

func (f *ViewFactory) NewCollectEndorsementsView(tx *Transaction, parties ...view.Identity) view.View {
	return f.collectEndorsementsView.New(tx.tx, parties...)
}

func (f *ViewFactory) NewCollectApprovesView(tx *Transaction, parties ...view.Identity) view.View {
	return f.collectEndorsementsView.New(tx.tx, parties...)
}

func (f *ViewFactory) RespondExchangeRecipientIdentities(context view.Context, opts ...ServiceOption) (view.Identity, view.Identity, error) {
	v, err := f.respondExchangeRecipientIdentitiesView.New(opts...)
	if err != nil {
		return nil, nil, err
	}
	ids, err := context.RunView(v)
	if err != nil {
		return nil, nil, err
	}

	return ids.([]view.Identity)[0], ids.([]view.Identity)[1], nil
}

func (f *ViewFactory) ExchangeRecipientIdentities(context view.Context, recipient view.Identity, opts ...ServiceOption) (view.Identity, view.Identity, error) {
	v, err := f.exchangeRecipientIdentitiesView.New(recipient, opts...)
	if err != nil {
		return nil, nil, err
	}
	ids, err := context.RunView(v)
	if err != nil {
		return nil, nil, err
	}

	return ids.([]view.Identity)[0], ids.([]view.Identity)[1], nil
}

func (f *ViewFactory) NewOrderingAndFinalityView(tx *Transaction) view.View {
	return f.orderingAndFinalityView.New(tx.tx, true)
}

func (f *ViewFactory) NewOrderingAndFinalityWithTimeoutView(tx *Transaction, timeout time.Duration) view.View {
	return f.orderingAndFinalityView.NewWithTimeout(tx.tx, true, timeout)
}

func (f *ViewFactory) ReceiveTransaction(context view.Context) (*Transaction, error) {
	txBoxed, err := context.RunView(f.receiveTransactionView.New(nil), view.WithSameContext())
	if err != nil {
		return nil, err
	}

	cctx := txBoxed.(*Transaction)
	return cctx, nil
}
