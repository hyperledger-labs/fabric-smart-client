/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"encoding/json"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	session2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/session"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

// RecipientData models the answer to a request of recipient identity
type RecipientData struct {
	// Identity is the recipient identity
	Identity view.Identity
}

// Bytes returns the byte representation of this struct
func (r *RecipientData) Bytes() ([]byte, error) {
	return json.Marshal(r)
}

// FromBytes unmarshalls the passed bytes into this struct
func (r *RecipientData) FromBytes(raw []byte) error {
	return json.Unmarshal(raw, r)
}

// ExchangeRecipientRequest models a request of exchange of recipient identities
type ExchangeRecipientRequest struct {
	Channel       string
	WalletID      []byte
	RecipientData *RecipientData
}

// Bytes returns the byte representation of this struct
func (r *ExchangeRecipientRequest) Bytes() ([]byte, error) {
	return json.Marshal(r)
}

// FromBytes unmarshalls the passed bytes into this struct
func (r *ExchangeRecipientRequest) FromBytes(raw []byte) error {
	return json.Unmarshal(raw, r)
}

// RecipientRequest models a request of recipient identity
type RecipientRequest struct {
	// Network identifier
	Network string
}

// Bytes returns the byte representation of this struct
func (r *RecipientRequest) Bytes() ([]byte, error) {
	return json.Marshal(r)
}

// FromBytes unmarshalls the passed bytes into this struct
func (r *RecipientRequest) FromBytes(raw []byte) error {
	return json.Unmarshal(raw, r)
}

type RequestRecipientIdentityView struct {
	Network string
	Other   view.Identity
}

func RequestRecipientIdentity(context view.Context, other view.Identity) (view.Identity, error) {
	recipientIdentityBoxed, err := context.RunView(&RequestRecipientIdentityView{Other: other})
	if err != nil {
		return nil, err
	}
	return recipientIdentityBoxed.(view.Identity), nil
}

func (f RequestRecipientIdentityView) Call(context view.Context) (interface{}, error) {
	session, err := context.GetSession(context.Initiator(), f.Other)
	if err != nil {
		return nil, err
	}

	// Ask for identity
	rr := &RecipientRequest{
		Network: f.Network,
	}
	rrRaw, err := rr.Bytes()
	if err != nil {
		return nil, errors.Wrapf(err, "failed marshalling recipient request")
	}
	err = session.Send(rrRaw)
	if err != nil {
		return nil, err
	}

	timeout := time.NewTimer(time.Minute)
	defer timeout.Stop()

	// Wait to receive an identity
	ch := session.Receive()
	var payload []byte
	select {
	case msg := <-ch:
		payload = msg.Payload
	case <-timeout.C:
		return nil, errors.New("time out reached")
	}

	recipientData := &RecipientData{}
	if err := recipientData.FromBytes(payload); err != nil {
		return nil, err
	}

	// Update the Endpoint Resolver
	if err := view2.GetEndpointService(context).Bind(context.Context(), f.Other, recipientData.Identity); err != nil {
		return nil, err
	}

	return recipientData.Identity, nil
}

// RespondRequestRecipientIdentityView models a view of a responder to a request of recipient identity
type RespondRequestRecipientIdentityView struct {
	Identity view.Identity
}

// Call does the following:
// 1. Reads a first message from the context's session
// 2. Unmarshall the message into rr = RecipientRequest
// 3. If the identity to send back is not set, it is set to fabric.GetFabricNetworkService(context, rr.Network).IdentityProvider().DefaultIdentity()
// 4. Send back marshalled RecipientData struct
func (s *RespondRequestRecipientIdentityView) Call(context view.Context) (interface{}, error) {
	session, payload, err := session2.ReadFirstMessage(context)
	if err != nil {
		return nil, err
	}

	rr := &RecipientRequest{}
	if err := rr.FromBytes(payload); err != nil {
		return nil, errors.Wrapf(err, "failed unmarshalling recipient request")
	}

	if s.Identity.IsNone() {
		fns, err := fabric.GetFabricNetworkService(context, rr.Network)
		if err != nil {
			return nil, err
		}
		s.Identity = fns.IdentityProvider().DefaultIdentity()
	}

	recipientData := &RecipientData{
		Identity: s.Identity,
	}
	recipientDataRaw, err := recipientData.Bytes()
	if err != nil {
		return nil, err
	}

	// Step 3: send the public key back to the invoker
	err = session.Send(recipientDataRaw)
	if err != nil {
		return nil, err
	}

	// Update the Endpoint Resolver
	resolver := view2.GetEndpointService(context)
	err = resolver.Bind(context.Context(), context.Me(), recipientData.Identity)
	if err != nil {
		return nil, err
	}

	return recipientData.Identity, nil
}

// NewRespondRequestRecipientIdentityView returns a new instance of RespondRequestRecipientIdentityView
func NewRespondRequestRecipientIdentityView() view.View {
	return &RespondRequestRecipientIdentityView{}
}

// RespondRequestRecipientIdentity runs the RespondRequestRecipientIdentityView and
// returns the identity sent to the requester. In this case, the identity used is the one returned by
// fabric.GetFabricNetworkService(context, rr.Network).IdentityProvider().DefaultIdentity()a
func RespondRequestRecipientIdentity(context view.Context) (view.Identity, error) {
	id, err := context.RunView(NewRespondRequestRecipientIdentityView())
	if err != nil {
		return nil, err
	}
	return id.(view.Identity), nil
}

// ExchangeRecipientIdentitiesView models the view of the initiator of an exchange of recipient identities.
type ExchangeRecipientIdentitiesView struct {
	Network       string
	IdentityLabel string
	Other         view.Identity
}

// ExchangeRecipientIdentities runs the ExchangeRecipientIdentitiesView against the passed receiver.
// The function returns, the recipient identity of the sender, the recipient identity of the receiver.
func ExchangeRecipientIdentities(context view.Context, recipient view.Identity, opts ...ServiceOption) (view.Identity, view.Identity, error) {
	opt, err := CompileServiceOptions(opts...)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to compile service options")
	}
	ids, err := context.RunView(&ExchangeRecipientIdentitiesView{
		Network:       opt.Network,
		Other:         recipient,
		IdentityLabel: opt.Identity,
	})
	if err != nil {
		return nil, nil, err
	}

	return ids.([]view.Identity)[0], ids.([]view.Identity)[1], nil
}

func (f *ExchangeRecipientIdentitiesView) Call(context view.Context) (interface{}, error) {
	session, err := context.GetSession(context.Initiator(), f.Other)
	if err != nil {
		return nil, err
	}

	fns, err := fabric.GetFabricNetworkService(context, f.Network)
	if err != nil {
		return nil, err
	}
	var me view.Identity
	if len(f.IdentityLabel) != 0 {
		me, err = fns.LocalMembership().GetIdentityByID(f.IdentityLabel)
		if err != nil {
			return nil, errors.WithMessagef(err, "failed to get identity with label %s", f.IdentityLabel)
		}
	} else {
		me = fns.LocalMembership().DefaultIdentity()
	}
	if me.IsNone() {
		return nil, errors.Errorf("no identity found with label %s", f.IdentityLabel)
	}

	// Send request
	request := &ExchangeRecipientRequest{
		WalletID: f.Other,
		RecipientData: &RecipientData{
			Identity: me,
		},
	}
	requestRaw, err := request.Bytes()
	if err != nil {
		return nil, err
	}
	if err := session.Send(requestRaw); err != nil {
		return nil, err
	}

	// Wait to receive an identity
	payload, err := session2.ReadMessageWithTimeout(session, 30*time.Second)
	if err != nil {
		return nil, err
	}

	recipientData := &RecipientData{}
	if err := recipientData.FromBytes(payload); err != nil {
		return nil, err
	}

	// Update the Endpoint Resolver
	logger.Debugf("bind [%s] to other [%s]", recipientData.Identity, f.Other)
	resolver := view2.GetEndpointService(context)
	err = resolver.Bind(context.Context(), f.Other, recipientData.Identity)
	if err != nil {
		return nil, err
	}

	logger.Debugf("bind me [%s] to [%s]", me, context.Me())
	err = resolver.Bind(context.Context(), context.Me(), me)
	if err != nil {
		return nil, err
	}

	return []view.Identity{me, recipientData.Identity}, nil
}

// RespondExchangeRecipientIdentitiesView models the view of the responder of an exchange of recipient identities.
type RespondExchangeRecipientIdentitiesView struct {
	Network       string
	IdentityLabel string
}

// RespondExchangeRecipientIdentities runs the RespondExchangeRecipientIdentitiesView
func RespondExchangeRecipientIdentities(context view.Context, opts ...ServiceOption) (view.Identity, view.Identity, error) {
	opt, err := CompileServiceOptions(opts...)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to compile service options")
	}
	ids, err := context.RunView(&RespondExchangeRecipientIdentitiesView{
		Network:       opt.Network,
		IdentityLabel: opt.Identity,
	})
	if err != nil {
		return nil, nil, err
	}

	return ids.([]view.Identity)[0], ids.([]view.Identity)[1], nil
}

func (s *RespondExchangeRecipientIdentitiesView) Call(context view.Context) (interface{}, error) {
	session, requestRaw, err := session2.ReadFirstMessage(context)
	if err != nil {
		return nil, err
	}

	// other
	request := &ExchangeRecipientRequest{}
	if err := request.FromBytes(requestRaw); err != nil {
		return nil, err
	}

	fns, err := fabric.GetFabricNetworkService(context, s.Network)
	if err != nil {
		return nil, err
	}
	var me view.Identity
	if len(s.IdentityLabel) != 0 {
		me, err = fns.LocalMembership().GetIdentityByID(s.IdentityLabel)
		if err != nil {
			return nil, errors.WithMessagef(err, "failed to get identity with label %s", s.IdentityLabel)
		}
	} else {
		me = fns.LocalMembership().DefaultIdentity()
	}
	if me.IsNone() {
		return nil, errors.Errorf("no identity found with label %s", s.IdentityLabel)
	}

	other := request.RecipientData.Identity

	recipientData := &RecipientData{
		Identity: me,
	}
	recipientDataRaw, err := recipientData.Bytes()
	if err != nil {
		return nil, err
	}

	if err := session.Send(recipientDataRaw); err != nil {
		return nil, err
	}

	// Update the Endpoint Resolver
	resolver := view2.GetEndpointService(context)
	err = resolver.Bind(context.Context(), context.Me(), me)
	if err != nil {
		return nil, err
	}
	err = resolver.Bind(context.Context(), session.Info().Caller, other)
	if err != nil {
		return nil, err
	}

	return []view.Identity{me, other}, nil
}
