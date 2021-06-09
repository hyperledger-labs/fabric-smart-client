/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package state

import (
	"encoding/json"
	"time"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	session2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/session"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type RecipientData struct {
	Identity view.Identity
}

func (r *RecipientData) Bytes() ([]byte, error) {
	return json.Marshal(r)
}

func (r *RecipientData) FromBytes(raw []byte) error {
	return json.Unmarshal(raw, r)
}

type ExchangeRecipientRequest struct {
	Channel       string
	WalletID      []byte
	RecipientData *RecipientData
}

func (r *ExchangeRecipientRequest) Bytes() ([]byte, error) {
	return json.Marshal(r)
}

func (r *ExchangeRecipientRequest) FromBytes(raw []byte) error {
	return json.Unmarshal(raw, r)
}

type RecipientRequest struct {
	Channel  string
	WalletID []byte
}

func (r *RecipientRequest) Bytes() ([]byte, error) {
	return json.Marshal(r)
}

func (r *RecipientRequest) FromBytes(raw []byte) error {
	return json.Unmarshal(raw, r)
}

type requestPseudonymView struct {
	Channel string
	Other   view.Identity
}

func RequestRecipientIdentity(context view.Context, other view.Identity) (view.Identity, error) {
	pseudonymBoxed, err := context.RunView(&requestPseudonymView{Other: other})
	if err != nil {
		return nil, err
	}
	return pseudonymBoxed.(view.Identity), nil
}

func (f requestPseudonymView) Call(context view.Context) (interface{}, error) {
	session, err := context.GetSession(context.Initiator(), f.Other)
	if err != nil {
		return nil, err
	}

	// Ask for identity
	rr := &RecipientRequest{
		Channel:  f.Channel,
		WalletID: f.Other,
	}
	rrRaw, err := rr.Bytes()
	if err != nil {
		return nil, errors.Wrapf(err, "failed marshalling recipient request")
	}
	err = session.Send(rrRaw)
	if err != nil {
		return nil, err
	}

	// Wait to receive an identity
	ch := session.Receive()
	var payload []byte
	select {
	case msg := <-ch:
		payload = msg.Payload
	case <-time.After(60 * time.Second):
		return nil, errors.New("time out reached")
	}

	recipientData := &RecipientData{}
	if err := recipientData.FromBytes(payload); err != nil {
		return nil, err
	}

	// Update the Endpoint Resolver
	if err := view2.GetEndpointService(context).Bind(f.Other, recipientData.Identity); err != nil {
		return nil, err
	}

	return recipientData.Identity, nil
}

type respondPseudonymView struct {
	Wallet string
}

func (s *respondPseudonymView) Call(context view.Context) (interface{}, error) {
	session, payload, err := session2.ReadFirstMessage(context)
	if err != nil {
		return nil, err
	}

	rr := &RecipientRequest{}
	if err := rr.FromBytes(payload); err != nil {
		return nil, errors.Wrapf(err, "failed unmarshalling recipient request")
	}

	recipientData := &RecipientData{
		Identity: fabric.GetDefaultNetwork(context).IdentityProvider().DefaultIdentity(),
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
	err = resolver.Bind(context.Me(), recipientData.Identity)
	if err != nil {
		return nil, err
	}

	return recipientData.Identity, nil
}

func NewRespondRequestRecipientIdentityView() view.View {
	return &respondPseudonymView{}
}

func RespondRequestRecipientIdentity(context view.Context) (view.Identity, error) {
	id, err := context.RunView(NewRespondRequestRecipientIdentityView())
	if err != nil {
		return nil, err
	}
	return id.(view.Identity), nil
}

type exchangePseudonymView struct {
	Network string
	Channel string
	Wallet  string
	Other   view.Identity
}

func (f *exchangePseudonymView) Call(context view.Context) (interface{}, error) {
	session, err := context.GetSession(context.Initiator(), f.Other)
	if err != nil {
		return nil, err
	}

	me := fabric.GetDefaultNetwork(context).IdentityProvider().DefaultIdentity()

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
	err = resolver.Bind(f.Other, recipientData.Identity)
	if err != nil {
		return nil, err
	}

	logger.Debugf("bind me [%s] to [%s]", me, context.Me())
	err = resolver.Bind(context.Me(), me)
	if err != nil {
		return nil, err
	}

	return []view.Identity{me, recipientData.Identity}, nil
}

type respondExchangePseudonymView struct {
	Wallet string
}

func (s *respondExchangePseudonymView) Call(context view.Context) (interface{}, error) {
	session, requestRaw, err := session2.ReadFirstMessage(context)
	if err != nil {
		return nil, err
	}

	// other
	request := &ExchangeRecipientRequest{}
	if err := request.FromBytes(requestRaw); err != nil {
		return nil, err
	}

	me := fabric.GetDefaultNetwork(context).IdentityProvider().DefaultIdentity()
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
	err = resolver.Bind(context.Me(), me)
	if err != nil {
		return nil, err
	}
	err = resolver.Bind(session.Info().Caller, other)
	if err != nil {
		return nil, err
	}

	return []view.Identity{me, other}, nil
}

func ExchangeRecipientIdentitiesInitiator(context view.Context, myWalletID string, recipient view.Identity) (view.Identity, view.Identity, error) {
	ids, err := context.RunView(&exchangePseudonymView{
		Channel: "",
		Wallet:  myWalletID,
		Other:   recipient,
	})
	if err != nil {
		return nil, nil, err
	}

	return ids.([]view.Identity)[0], ids.([]view.Identity)[1], nil
}

func ExchangeRecipientIdentitiesResponder(context view.Context) (view.Identity, view.Identity, error) {
	ids, err := context.RunView(&respondExchangePseudonymView{})
	if err != nil {
		return nil, nil, err
	}

	return ids.([]view.Identity)[0], ids.([]view.Identity)[1], nil
}
