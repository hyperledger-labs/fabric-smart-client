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
	commsession "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/session"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/security"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// RecipientData carries a recipient identity and the sender's proof that it
// controls that identity.
type RecipientData struct {
	// Identity is the recipient identity
	Identity view.Identity
	// Proof is Identity's signature over the ProofContext of the message that
	// carries this RecipientData; see SignRecipientProof. The receiver refuses
	// Identity if Proof is missing or does not verify.
	Proof []byte `json:",omitempty"`
}

// Bytes returns the byte representation of this struct
func (r *RecipientData) Bytes() ([]byte, error) {
	return json.Marshal(r)
}

// FromBytes unmarshals the passed bytes into this struct
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

// FromBytes unmarshals the passed bytes into this struct
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

// FromBytes unmarshals the passed bytes into this struct
func (r *RecipientRequest) FromBytes(raw []byte) error {
	return json.Unmarshal(raw, r)
}

// RequestRecipientIdentityView asks Other for a recipient identity, verifies
// the proof of ownership that comes with it, and binds the identity to Other's
// FSC node identity.
type RequestRecipientIdentityView struct {
	Network string
	Other   view.Identity
}

func RequestRecipientIdentity(viewCtx view.Context, other view.Identity) (view.Identity, error) {
	recipientIdentityBoxed, err := viewCtx.RunView(&RequestRecipientIdentityView{Other: other})
	if err != nil {
		return nil, err
	}
	id, ok := recipientIdentityBoxed.(view.Identity)
	if !ok {
		return nil, errors.Errorf("unexpected view result type [%T]", recipientIdentityBoxed)
	}
	return id, nil
}

func (f RequestRecipientIdentityView) Call(viewCtx view.Context) (any, error) {
	session, err := viewCtx.GetSession(viewCtx.Initiator(), f.Other)
	if err != nil {
		return nil, err
	}
	fns, err := fabric.GetFabricNetworkService(viewCtx, f.Network)
	if err != nil {
		return nil, err
	}
	own, other, err := initiatorNodes(viewCtx, session, f.Other)
	if err != nil {
		return nil, err
	}

	// Ask for identity
	rr := &RecipientRequest{
		Network: f.Network,
	}
	rrRaw, err := rr.Bytes()
	if err != nil {
		return nil, errors.Wrap(err, "failed marshalling recipient request")
	}
	err = session.Send(viewCtx.Context(), rrRaw)
	if err != nil {
		return nil, err
	}

	// Wait to receive an identity
	payload, err := commsession.ReadMessageWithTimeout(session, time.Minute)
	if err != nil {
		return nil, err
	}

	recipientData := &RecipientData{}
	if err := recipientData.FromBytes(payload); err != nil {
		return nil, err
	}

	p := proofContext(TagRequestReply, viewCtx, session, other, own, recipientData.Identity)
	if err := verifyProof(viewCtx, fns, p, recipientData.Proof); err != nil {
		return nil, err
	}

	// Update the Endpoint Resolver
	if err := endpoint.GetService(viewCtx).BindProven(viewCtx.Context(), other, recipientData.Identity); err != nil {
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
// 2. Unmarshal the message into rr = RecipientRequest
// 3. If the identity to send back is not set, it is set to fabric.GetFabricNetworkService(context, rr.Network).IdentityProvider().DefaultIdentity()
// 4. Signs a proof of ownership of the identity with the network's signer for it
// 5. Send back marshalled RecipientData struct
func (s *RespondRequestRecipientIdentityView) Call(viewCtx view.Context) (any, error) {
	session, payload, err := commsession.ReadFirstMessage(viewCtx)
	if err != nil {
		return nil, err
	}

	rr := &RecipientRequest{}
	if err := rr.FromBytes(payload); err != nil {
		return nil, errors.Wrap(err, "failed unmarshalling recipient request")
	}

	fns, err := fabric.GetFabricNetworkService(viewCtx, rr.Network)
	if err != nil {
		return nil, err
	}
	if s.Identity.IsNone() {
		s.Identity = fns.IdentityProvider().DefaultIdentity()
	}
	own, err := ownNode(viewCtx)
	if err != nil {
		return nil, err
	}
	proof, err := signProof(fns, proofContext(TagRequestReply, viewCtx, session, own, session.Info().Caller, s.Identity))
	if err != nil {
		return nil, err
	}

	recipientData := &RecipientData{
		Identity: s.Identity,
		Proof:    proof,
	}
	recipientDataRaw, err := recipientData.Bytes()
	if err != nil {
		return nil, err
	}

	// Step 3: send the public key back to the invoker
	err = session.Send(viewCtx.Context(), recipientDataRaw)
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
	res, err := viewCtx.RunView(NewRespondRequestRecipientIdentityView())
	if err != nil {
		return nil, err
	}
	id, ok := res.(view.Identity)
	if !ok {
		return nil, errors.Errorf("unexpected view result type [%T]", res)
	}
	return id, nil
}

// ExchangeRecipientIdentitiesView models the view of the initiator of an exchange of recipient identities.
type ExchangeRecipientIdentitiesView struct {
	Network       string
	IdentityLabel string
	Other         view.Identity
}

// ExchangeRecipientIdentities runs the ExchangeRecipientIdentitiesView against the passed receiver.
// The function returns, the recipient identity of the sender, the recipient identity of the receiver.
func ExchangeRecipientIdentities(viewCtx view.Context, recipient view.Identity, opts ...ServiceOption) (sender, receiver view.Identity, err error) {
	opt, err := CompileServiceOptions(opts...)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to compile service options")
	}
	ids, err := viewCtx.RunView(&ExchangeRecipientIdentitiesView{
		Network:       opt.Network,
		Other:         recipient,
		IdentityLabel: opt.Identity,
	})
	if err != nil {
		return nil, nil, err
	}

	idList, ok := ids.([]view.Identity)
	if !ok {
		return nil, nil, errors.Errorf("unexpected view result type [%T]", ids)
	}
	if len(idList) != 2 {
		return nil, nil, errors.Errorf("expected 2 identities, got [%d]", len(idList))
	}
	return idList[0], idList[1], nil
}

func (f *ExchangeRecipientIdentitiesView) Call(viewCtx view.Context) (any, error) {
	session, err := viewCtx.GetSession(viewCtx.Initiator(), f.Other)
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
	own, other, err := initiatorNodes(viewCtx, session, f.Other)
	if err != nil {
		return nil, err
	}
	proof, err := signProof(fns, proofContext(TagExchangeRequest, viewCtx, session, own, other, me))
	if err != nil {
		return nil, err
	}

	// Send request
	request := &ExchangeRecipientRequest{
		WalletID: f.Other,
		RecipientData: &RecipientData{
			Identity: me,
			Proof:    proof,
		},
	}
	requestRaw, err := request.Bytes()
	if err != nil {
		return nil, err
	}
	if err := session.Send(viewCtx.Context(), requestRaw); err != nil {
		return nil, err
	}

	// Wait to receive an identity
	payload, err := commsession.ReadMessageWithTimeout(session, 30*time.Second)
	if err != nil {
		return nil, err
	}

	recipientData := &RecipientData{}
	if err := recipientData.FromBytes(payload); err != nil {
		return nil, err
	}

	p := proofContext(TagExchangeReply, viewCtx, session, other, own, recipientData.Identity)
	if err := verifyProof(viewCtx, fns, p, recipientData.Proof); err != nil {
		return nil, err
	}

	// Update the Endpoint Resolver
	logger.Debugf("bind [%s] to other [%s]", recipientData.Identity, other)
	resolver := endpoint.GetService(viewCtx)
	err = resolver.BindProven(viewCtx.Context(), other, recipientData.Identity)
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
func RespondExchangeRecipientIdentities(viewCtx view.Context, opts ...ServiceOption) (sender, receiver view.Identity, err error) {
	opt, err := CompileServiceOptions(opts...)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to compile service options")
	}
	ids, err := viewCtx.RunView(&RespondExchangeRecipientIdentitiesView{
		Network:       opt.Network,
		IdentityLabel: opt.Identity,
	})
	if err != nil {
		return nil, nil, err
	}

	idList, ok := ids.([]view.Identity)
	if !ok {
		return nil, nil, errors.Errorf("unexpected view result type [%T]", ids)
	}
	if len(idList) != 2 {
		return nil, nil, errors.Errorf("expected 2 identities, got [%d]", len(idList))
	}
	return idList[0], idList[1], nil
}

// Call does the following:
//  1. Reads a first message from the context's session and unmarshals it into an ExchangeRecipientRequest
//  2. Rejects the request if it does not carry recipient data, since the message comes from the counterparty
//     and cannot be trusted to be well-formed
//  3. Resolves the identity to send back, defaulting to fns.LocalMembership().DefaultIdentity() if IdentityLabel is not set
//  4. Verifies the initiator's proof of ownership of its identity; on failure it replies with an
//     error carrying the reason and binds nothing
//  5. Sends back the resolved identity with its own proof and binds both identities in the Endpoint Resolver
func (s *RespondExchangeRecipientIdentitiesView) Call(viewCtx view.Context) (any, error) {
	session, requestRaw, err := commsession.ReadFirstMessage(viewCtx)
	if err != nil {
		return nil, err
	}

	// other
	request := &ExchangeRecipientRequest{}
	if err := request.FromBytes(requestRaw); err != nil {
		return nil, err
	}
	// RecipientData is attacker-controlled: the counterparty can send a well-formed
	// request that omits it, so it must be checked before use.
	if request.RecipientData == nil {
		return nil, errors.New("missing recipient data in request")
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

	other := request.RecipientData.Identity
	caller := session.Info().Caller
	own, err := ownNode(viewCtx)
	if err != nil {
		return nil, err
	}

	// Verify before replying, so a refused initiator learns why and gets no identity.
	p := proofContext(TagExchangeRequest, viewCtx, session, caller, own, other)
	if err := verifyProof(viewCtx, fns, p, request.RecipientData.Proof); err != nil {
		if sendErr := session.SendError(viewCtx.Context(), []byte(err.Error())); sendErr != nil {
			logger.Debugf("failed sending recipient proof error to [%s]: %v", caller, sendErr)
		}
		return nil, err
	}

	proof, err := signProof(fns, proofContext(TagExchangeReply, viewCtx, session, own, caller, me))
	if err != nil {
		return nil, err
	}
	recipientData := &RecipientData{
		Identity: me,
		Proof:    proof,
	}
	recipientDataRaw, err := recipientData.Bytes()
	if err != nil {
		return nil, err
	}

	if err := session.Send(viewCtx.Context(), recipientDataRaw); err != nil {
		return nil, err
	}

	// Update the Endpoint Resolver
	resolver := endpoint.GetService(viewCtx)
	err = resolver.Bind(viewCtx.Context(), viewCtx.Me(), me)
	if err != nil {
		return nil, err
	}
	err = resolver.BindProven(viewCtx.Context(), caller, other)
	if err != nil {
		return nil, err
	}

	return []view.Identity{me, other}, nil
}

// ownNode returns this node's FSC identity, the one remote nodes see as the
// Caller of sessions it opens.
func ownNode(viewCtx view.Context) (view.Identity, error) {
	p, err := id.GetProvider(viewCtx)
	if err != nil {
		return nil, errors.WithMessage(err, "failed getting node identity provider")
	}
	return p.DefaultIdentity(), nil
}

// initiatorNodes returns this node's FSC identity and that of the node session
// reaches. The latter is the resolver identity of the transport-verified
// RemotePKID, or of party if the session has none. Bindings are made to that
// identity because the endpoint service follows a single binding hop to a
// resolver.
func initiatorNodes(viewCtx view.Context, session view.Session, party view.Identity) (own, other view.Identity, err error) {
	own, err = ownNode(viewCtx)
	if err != nil {
		return nil, nil, err
	}
	es := endpoint.GetService(viewCtx)
	if pkid := session.Info().RemotePKID; len(pkid) != 0 {
		other, err = es.GetIdentity("", pkid)
	} else {
		var r *endpoint.Resolver
		if r, err = es.GetResolver(viewCtx.Context(), party); err == nil {
			other = r.GetId()
		}
	}
	if err != nil {
		return nil, nil, errors.WithMessagef(err, "failed resolving node identity of [%s]", party)
	}
	return own, other, nil
}

func proofContext(tag ProofTag, viewCtx view.Context, session view.Session, signerNode, verifierNode, x view.Identity) ProofContext {
	return ProofContext{
		Tag:          tag,
		SessionID:    session.Info().ID,
		ContextID:    viewCtx.ID(),
		SignerNode:   signerNode,
		VerifierNode: verifierNode,
		Identity:     x,
	}
}

// signProof signs p with the network's signer for p.Identity.
func signProof(fns *fabric.NetworkService, p ProofContext) ([]byte, error) {
	signer, err := fns.SignerService().GetSigner(p.Identity)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed getting signer for recipient identity [%s]", p.Identity)
	}
	return SignRecipientProof(signer, p)
}

// Reasons verifyProof reports a refused recipient proof with. They are fixed
// strings, so the warn line carries no bytes the counterparty chose.
const (
	reasonMissing           = "missing"
	reasonIncompleteContext = "incomplete context"
	reasonNoVerifier        = "no verifier"
	reasonInvalidSignature  = "invalid signature"
)

// verifyProof verifies proof against p with the network's verifier for
// p.Identity, and reports a failure as security.KindRecipientProof with one
// of the reason* constants.
func verifyProof(viewCtx view.Context, fns *fabric.NetworkService, p ProofContext, proof []byte) error {
	reason, err := checkProof(fns, p, proof)
	if err != nil {
		security.GetRejections(viewCtx).Report(viewCtx.Context(), security.KindRecipientProof, p.SignerNode, "", reason)
	}
	return err
}

func checkProof(fns *fabric.NetworkService, p ProofContext, proof []byte) (string, error) {
	if len(proof) == 0 {
		return reasonMissing, VerifyRecipientProof(nil, p, proof)
	}
	if !p.complete() {
		return reasonIncompleteContext, VerifyRecipientProof(nil, p, proof)
	}
	verifier, err := fns.SignerService().GetVerifier(p.Identity)
	if err != nil {
		return reasonNoVerifier, errors.Errorf("%w: no verifier for recipient identity: %s", ErrRecipientProof, err.Error())
	}
	if err := VerifyRecipientProof(verifier, p, proof); err != nil {
		return reasonInvalidSignature, err
	}
	return "", nil
}
