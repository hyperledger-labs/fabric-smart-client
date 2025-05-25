/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

type collectEndorsementsView struct {
	tx                *Transaction
	parties           []view.Identity
	deleteTransient   bool
	verifierProviders []fabric.VerifierProvider
}

func (c *collectEndorsementsView) Call(context view.Context) (interface{}, error) {
	span := trace.SpanFromContext(context.Context())
	// Prepare verifiers
	ch, err := c.tx.FabricNetworkService().Channel(c.tx.Channel())
	if err != nil {
		return nil, errors.Wrapf(err, "failed getting channel [%s:%s]", c.tx.Network(), c.tx.Channel())
	}
	mspManager := ch.MSPManager()

	var vProviders []fabric.VerifierProvider
	vProviders = append(vProviders, c.verifierProviders...)
	vProviders = append(vProviders, c.tx.verifierProviders...)
	vProviders = append(vProviders, &verifierProviderWrapper{m: mspManager})

	// Get results to send
	span.AddEvent("Get results to send")
	res, err := c.tx.Results()
	if err != nil {
		return nil, errors.Wrapf(err, "failed getting tx results")
	}

	// Contact sequantially all parties.
	logger.Debugf("Collect Endorsements from [%d] parties [%v]", len(c.parties), c.parties)
	span.AddEvent("Start collecting endorsement")
	for _, party := range c.parties {
		span.AddEvent("Collect endorsement iteration")
		logger.Debugf("Collect Endorsements On Simulation from [%s]", party)

		var err error
		if context.IsMe(party) {
			span.AddEvent("Start endorsing locally")
			logger.Debugf("This is me %s, endorse locally.", party)
			// Endorse it
			err = c.tx.EndorseWithIdentity(party)
			if err != nil {
				return nil, errors.Wrap(err, "failed endorsing transaction")
			}
			span.AddEvent("Finish endorsing locally")
			continue
		}

		var txRaw []byte
		span.AddEvent("Get transaction bytes")
		if c.deleteTransient {
			txRaw, err = c.tx.BytesNoTransient()
			if err != nil {
				return nil, errors.Wrap(err, "failed marshalling transaction content")
			}
		} else {
			txRaw, err = c.tx.Bytes()
			if err != nil {
				return nil, errors.Wrap(err, "failed marshalling transaction content")
			}
		}

		span.AddEvent("Get session with party")
		session, err := context.GetSession(context.Initiator(), party)
		if err != nil {
			return nil, errors.Wrap(err, "failed getting session")
		}

		// Get a channel to receive the answer
		span.AddEvent("Start receiving from party")
		ch := session.Receive()

		// Send transaction
		span.AddEvent("Send raw transaction to party")
		err = session.SendWithContext(context.Context(), txRaw)
		if err != nil {
			return nil, errors.Wrap(err, "failed sending transaction content")
		}

		timeout := time.NewTimer(time.Minute)

		span.AddEvent("Wait for transaction")
		// Wait for the answer
		var msg *view.Message
		select {
		case msg = <-ch:
			timeout.Stop()
		case <-timeout.C:
			timeout.Stop()
			return nil, errors.Errorf("Timeout from party %s", party)
		}
		span.AddEvent("Received transaction from party")
		if msg.Status == view.ERROR {
			return nil, errors.New(string(msg.Payload))
		}

		logger.Debugf("got response from party [%s]", party)

		// The response contains an array of marshalled ProposalResponse message
		var responses [][]byte
		if err := json.Unmarshal(msg.Payload, &responses); err != nil {
			return nil, errors.Wrapf(err, "failed unmarshalling response")
		}

		found := true
		fns, err := fabric.GetFabricNetworkService(context, c.tx.Network())
		if err != nil {
			return nil, errors.WithMessagef(err, "fabric network service [%s] not found", c.tx.Network())
		}
		tm := fns.TransactionManager()
		for _, response := range responses {
			span.AddEvent("Parse proposal response")
			proposalResponse, err := tm.NewProposalResponseFromBytes(response)
			if err != nil {
				return nil, errors.Wrap(err, "failed unmarshalling received proposal response")
			}

			endorser := view.Identity(proposalResponse.Endorser())

			// Check the validity of the response
			span.AddEvent("Check response validity")
			if view2.GetEndpointService(context).IsBoundTo(context.Context(), endorser, party) {
				found = true
			}

			// TODO: check the verifier providers, if any
			verified := false
			span.AddEvent("Start verification with providers")
			for _, provider := range vProviders {
				span.AddEvent("Verify endorsement")
				err := proposalResponse.VerifyEndorsement(provider)
				span.AddEvent("Verified endorsement")
				if err == nil {
					logger.Debugf("endorsement [%s] is valid", endorser)
					verified = true
					break
				}
				logger.Debugf("endorsement [%s] is invalid, reason [%s]", endorser, err)
			}
			if !verified {
				return nil, errors.Errorf("failed to verify signature for party [%s][%s]", endorser.String(), string(endorser))
			}
			// Check the content of the response
			// Now results can be equal to what this node has proposed or different
			if !bytes.Equal(res, proposalResponse.Results()) {
				return nil, errors.Errorf("received different results")
			}

			logger.Debugf("append response from party [%s]", party)
			span.AddEvent("Append proposal response")
			err = c.tx.AppendProposalResponse(proposalResponse)
			span.AddEvent("Appended proposal response")
			if err != nil {
				return nil, errors.Wrap(err, "failed appending received proposal response")
			}
		}

		if !found {
			return nil, errors.Errorf("invalid endorsement, expected one signed by [%s]", party.String())
		}
	}
	return c.tx, nil
}

func (c *collectEndorsementsView) SetVerifierProviders(p []fabric.VerifierProvider) *collectEndorsementsView {
	c.verifierProviders = p
	return c
}

func NewCollectEndorsementsView(tx *Transaction, parties ...view.Identity) *collectEndorsementsView {
	return &collectEndorsementsView{tx: tx, parties: parties}
}

func NewCollectApprovesView(tx *Transaction, parties ...view.Identity) *collectEndorsementsView {
	return &collectEndorsementsView{tx: tx, parties: parties, deleteTransient: true}
}

type endorseView struct {
	tx         *Transaction
	identities []view.Identity
}

func (s *endorseView) Call(context view.Context) (interface{}, error) {
	if len(s.identities) == 0 {
		fns, err := fabric.GetFabricNetworkService(context, s.tx.Network())
		if err != nil {
			return nil, errors.WithMessagef(err, "fabric network service [%s] not found", s.tx.Network())
		}
		s.identities = []view.Identity{fns.IdentityProvider().DefaultIdentity()}
	}

	var responses [][]byte
	for _, id := range s.identities {
		err := s.tx.EndorseWithIdentity(id)
		if err != nil {
			return nil, err
		}

		pr, err := s.tx.ProposalResponse()
		if err != nil {
			return nil, err
		}
		responses = append(responses, pr)
	}

	txRaw, err := s.tx.Bytes()
	if err != nil {
		return nil, errors.Wrap(err, "failed marshalling tx")
	}
	fns, err := fabric.GetDefaultFNS(context)
	if err != nil {
		return nil, errors.WithMessage(err, "failed getting default network")
	}
	ch, err := fns.Channel(s.tx.Channel())
	if err != nil {
		return nil, errors.WithMessagef(err, "failed getting channel [%s]", s.tx.Channel())
	}
	err = ch.Vault().StoreTransaction(context.Context(), s.tx.ID(), txRaw)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed storing tx env [%s]", s.tx.ID())
	}

	// Send the proposal response back
	raw, err := json.Marshal(responses)
	if err != nil {
		return nil, err
	}

	err = context.Session().SendWithContext(context.Context(), raw)
	if err != nil {
		return nil, err
	}

	return s.tx, nil
}

func NewEndorseView(tx *Transaction, ids ...view.Identity) *endorseView {
	return &endorseView{tx: tx, identities: ids}
}

func NewAcceptView(tx *Transaction, ids ...view.Identity) *endorseView {
	return &endorseView{tx: tx, identities: ids}
}

type verifierProviderWrapper struct {
	m *fabric.MSPManager
}

func (v *verifierProviderWrapper) GetVerifier(identity view.Identity) (fabric.Verifier, error) {
	return v.m.GetVerifier(identity)
}
