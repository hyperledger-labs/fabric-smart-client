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
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/session"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// recipientTimeout bounds how long ForgedRecipientView waits for the responder's reply,
// as long as the honest exchange initiator waits.
const recipientTimeout = 30 * time.Second

// Recipient is the input of HonestExchangeView and ForgedRecipientView.
type Recipient struct {
	// Responder is the FSC node to exchange recipient identities with.
	Responder view.Identity
	// Claim is the recipient identity ForgedRecipientView claims as its own.
	// HonestExchangeView ignores it.
	Claim view.Identity
}

// Exchanged is what HonestExchangeView returns.
type Exchanged struct {
	// Sender is the initiator's recipient identity.
	Sender view.Identity
	// Receiver is the responder's recipient identity.
	Receiver view.Identity
}

// HonestExchangeView exchanges recipient identities with the responder, each side
// proving ownership of its own identity.
type HonestExchangeView struct {
	Recipient
}

func (v *HonestExchangeView) Call(viewCtx view.Context) (any, error) {
	sender, receiver, err := state.ExchangeRecipientIdentities(viewCtx, v.Responder)
	if err != nil {
		return nil, err
	}
	return &Exchanged{Sender: sender, Receiver: receiver}, nil
}

// HonestExchangeViewFactory instantiates HonestExchangeView from JSON input.
type HonestExchangeViewFactory struct{}

func (*HonestExchangeViewFactory) NewView(in []byte) (view.View, error) {
	f := &HonestExchangeView{}
	if err := json.Unmarshal(in, &f.Recipient); err != nil {
		return nil, err
	}
	return f, nil
}

// RecipientRefused is what ForgedRecipientView returns when the responder refuses the
// claimed identity.
type RecipientRefused struct {
	// Reason is the error the responder replied with.
	Reason string
}

// ForgedRecipientView opens a recipient exchange with the responder and claims Claim,
// an identity it does not own, without a proof of ownership. It returns RecipientRefused
// when the responder replies with an error, and an error when the responder accepts the
// claim. It has its own type, so the honest RecipientResponderView can be registered for
// it next to HonestExchangeView.
type ForgedRecipientView struct {
	Recipient
}

func (v *ForgedRecipientView) Call(viewCtx view.Context) (any, error) {
	s, err := viewCtx.GetSession(viewCtx.Initiator(), v.Responder)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed getting session to [%s]", v.Responder)
	}
	request := &state.ExchangeRecipientRequest{
		WalletID:      v.Responder,
		RecipientData: &state.RecipientData{Identity: v.Claim},
	}
	raw, err := request.Bytes()
	if err != nil {
		return nil, err
	}
	if err := s.Send(viewCtx.Context(), raw); err != nil {
		return nil, errors.WithMessage(err, "failed sending the exchange request")
	}
	_, err = session.ReadMessageWithContext(viewCtx.Context(), s, recipientTimeout)
	if err == nil {
		return nil, errors.Errorf("responder accepted recipient identity [%s] claimed without proof", v.Claim)
	}
	return &RecipientRefused{Reason: err.Error()}, nil
}

// ForgedRecipientViewFactory instantiates ForgedRecipientView from JSON input.
type ForgedRecipientViewFactory struct{}

func (*ForgedRecipientViewFactory) NewView(in []byte) (view.View, error) {
	f := &ForgedRecipientView{}
	if err := json.Unmarshal(in, &f.Recipient); err != nil {
		return nil, err
	}
	return f, nil
}

// RecipientResponderView answers a recipient exchange with the production responder.
type RecipientResponderView struct{}

func (*RecipientResponderView) Call(viewCtx view.Context) (any, error) {
	_, _, err := state.RespondExchangeRecipientIdentities(viewCtx)
	return nil, err
}

// RecipientOwner is the input of RecipientOwnerView.
type RecipientOwner struct {
	// Identity is the recipient identity to resolve. When empty, RecipientOwnerView
	// resolves the node's own default identity.
	Identity view.Identity
}

// Owner is what RecipientOwnerView returns.
type Owner struct {
	// Identity is the recipient identity that was resolved.
	Identity view.Identity
	// Name is the name of the FSC node the identity resolves to.
	Name string
}

// RecipientOwnerView reports which FSC node the node's endpoint service routes a
// recipient identity to.
type RecipientOwnerView struct {
	RecipientOwner
}

func (v *RecipientOwnerView) Call(viewCtx view.Context) (any, error) {
	id := v.Identity
	if id.IsNone() {
		fns, err := fabric.GetDefaultFNS(viewCtx)
		if err != nil {
			return nil, err
		}
		id = fns.LocalMembership().DefaultIdentity()
	}
	resolver, err := endpoint.GetService(viewCtx).GetResolver(viewCtx.Context(), id)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed resolving [%s]", id)
	}
	return &Owner{Identity: id, Name: resolver.GetName()}, nil
}

// RecipientOwnerViewFactory instantiates RecipientOwnerView from JSON input.
type RecipientOwnerViewFactory struct{}

func (*RecipientOwnerViewFactory) NewView(in []byte) (view.View, error) {
	f := &RecipientOwnerView{}
	if err := json.Unmarshal(in, &f.RecipientOwner); err != nil {
		return nil, err
	}
	return f, nil
}
