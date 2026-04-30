/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"encoding/json"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	session2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/session"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
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
	// WalletID optionally identifies the recipient account to resolve on the remote node.
	WalletID []byte
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
	Node    view.Identity
	Wallet  view.Identity
}

func RequestRecipientIdentity(viewCtx view.Context, other view.Identity) (view.Identity, error) {
	recipientIdentityBoxed, err := viewCtx.RunView(&RequestRecipientIdentityView{Other: other})
	if err != nil {
		return nil, err
	}
	return recipientIdentityBoxed.(view.Identity), nil
}

// RequestRecipientIdentityFromNode requests a recipient identity from the passed node for the passed wallet alias.
func RequestRecipientIdentityFromNode(viewCtx view.Context, node, wallet view.Identity, opts ...ServiceOption) (view.Identity, error) {
	opt, err := CompileServiceOptions(opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to compile service options")
	}
	recipientIdentityBoxed, err := viewCtx.RunView(&RequestRecipientIdentityView{
		Network: opt.Network,
		Node:    node,
		Wallet:  wallet,
	})
	if err != nil {
		return nil, err
	}
	return recipientIdentityBoxed.(view.Identity), nil
}

func (f RequestRecipientIdentityView) remoteNode() view.Identity {
	if !f.Node.IsNone() {
		return f.Node
	}
	return f.Other
}

func (f RequestRecipientIdentityView) requestedWallet() view.Identity {
	return f.Wallet
}

func (f RequestRecipientIdentityView) Call(viewCtx view.Context) (interface{}, error) {
	remoteNode := f.remoteNode()
	session, err := viewCtx.GetSession(viewCtx.Initiator(), remoteNode)
	if err != nil {
		return nil, err
	}

	// Ask for identity
	rr := &RecipientRequest{
		Network:  f.Network,
		WalletID: f.requestedWallet(),
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
	if err := endpoint.GetService(viewCtx).Bind(viewCtx.Context(), remoteNode, recipientData.Identity); err != nil {
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
func (s *RespondRequestRecipientIdentityView) Call(viewCtx view.Context) (interface{}, error) {
	session, payload, err := session2.ReadFirstMessage(viewCtx)
	if err != nil {
		return nil, err
	}

	rr := &RecipientRequest{}
	if err := rr.FromBytes(payload); err != nil {
		return nil, errors.Wrapf(err, "failed unmarshalling recipient request")
	}

	if s.Identity.IsNone() {
		fns, err := fabric.GetFabricNetworkService(viewCtx, rr.Network)
		if err != nil {
			return nil, err
		}
		if !view.Identity(rr.WalletID).IsNone() {
			s.Identity, err = fns.LocalMembership().GetIdentityByID(string(rr.WalletID))
			if err != nil {
				return nil, errors.WithMessagef(err, "failed to get identity with label %s", string(rr.WalletID))
			}
		} else {
			s.Identity = fns.IdentityProvider().DefaultIdentity()
		}
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
	resolver := endpoint.GetService(viewCtx)
	err = resolver.Bind(viewCtx.Context(), viewCtx.Me(), recipientData.Identity)
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
func RespondRequestRecipientIdentity(viewCtx view.Context) (view.Identity, error) {
	id, err := viewCtx.RunView(NewRespondRequestRecipientIdentityView())
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
	Node          view.Identity
	Wallet        view.Identity
}

// ExchangeRecipientIdentities runs the ExchangeRecipientIdentitiesView against the passed receiver.
// The function returns, the recipient identity of the sender, the recipient identity of the receiver.
func ExchangeRecipientIdentities(viewCtx view.Context, recipient view.Identity, opts ...ServiceOption) (view.Identity, view.Identity, error) {
	opt, err := CompileServiceOptions(opts...)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to compile service options")
	}
	ids, err := viewCtx.RunView(&ExchangeRecipientIdentitiesView{
		Network:       opt.Network,
		Other:         recipient,
		IdentityLabel: opt.Identity,
	})
	if err != nil {
		return nil, nil, err
	}

	return ids.([]view.Identity)[0], ids.([]view.Identity)[1], nil
}

// ExchangeRecipientIdentitiesWithNode exchanges recipient identities with the passed node for the passed wallet alias.
func ExchangeRecipientIdentitiesWithNode(viewCtx view.Context, node, wallet view.Identity, opts ...ServiceOption) (view.Identity, view.Identity, error) {
	opt, err := CompileServiceOptions(opts...)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to compile service options")
	}
	ids, err := viewCtx.RunView(&ExchangeRecipientIdentitiesView{
		Network:       opt.Network,
		IdentityLabel: opt.Identity,
		Node:          node,
		Wallet:        wallet,
	})
	if err != nil {
		return nil, nil, err
	}

	return ids.([]view.Identity)[0], ids.([]view.Identity)[1], nil
}

func (f *ExchangeRecipientIdentitiesView) remoteNode() view.Identity {
	if !f.Node.IsNone() {
		return f.Node
	}
	return f.Other
}

func (f *ExchangeRecipientIdentitiesView) requestedWallet() view.Identity {
	return f.Wallet
}

func (f *ExchangeRecipientIdentitiesView) Call(viewCtx view.Context) (interface{}, error) {
	remoteNode := f.remoteNode()
	session, err := viewCtx.GetSession(viewCtx.Initiator(), remoteNode)
	if err != nil {
		return nil, err
	}

	fns, err := fabric.GetFabricNetworkService(viewCtx, f.Network)
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
		RecipientData: &RecipientData{
			Identity: me,
		},
	}
	if !f.requestedWallet().IsNone() {
		request.WalletID = f.requestedWallet()
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
	logger.Debugf("bind [%s] to other [%s]", recipientData.Identity, remoteNode)
	resolver := endpoint.GetService(viewCtx)
	err = resolver.Bind(viewCtx.Context(), remoteNode, recipientData.Identity)
	if err != nil {
		return nil, err
	}

	logger.Debugf("bind me [%s] to [%s]", me, viewCtx.Me())
	err = resolver.Bind(viewCtx.Context(), viewCtx.Me(), me)
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
func RespondExchangeRecipientIdentities(viewCtx view.Context, opts ...ServiceOption) (view.Identity, view.Identity, error) {
	opt, err := CompileServiceOptions(opts...)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to compile service options")
	}
	ids, err := viewCtx.RunView(&RespondExchangeRecipientIdentitiesView{
		Network:       opt.Network,
		IdentityLabel: opt.Identity,
	})
	if err != nil {
		return nil, nil, err
	}

	return ids.([]view.Identity)[0], ids.([]view.Identity)[1], nil
}

func (s *RespondExchangeRecipientIdentitiesView) Call(viewCtx view.Context) (interface{}, error) {
	session, requestRaw, err := session2.ReadFirstMessage(viewCtx)
	if err != nil {
		return nil, err
	}

	// other
	request := &ExchangeRecipientRequest{}
	if err := request.FromBytes(requestRaw); err != nil {
		return nil, err
	}

	fns, err := fabric.GetFabricNetworkService(viewCtx, s.Network)
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
	if len(request.WalletID) != 0 && len(s.IdentityLabel) == 0 {
		me, err = fns.LocalMembership().GetIdentityByID(string(request.WalletID))
		if err != nil {
			return nil, errors.WithMessagef(err, "failed to get identity with label %s", string(request.WalletID))
		}
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
	resolver := endpoint.GetService(viewCtx)
	err = resolver.Bind(viewCtx.Context(), viewCtx.Me(), me)
	if err != nil {
		return nil, err
	}
	err = resolver.Bind(viewCtx.Context(), session.Info().Caller, other)
	if err != nil {
		return nil, err
	}

	return []view.Identity{me, other}, nil
}
