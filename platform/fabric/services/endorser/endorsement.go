/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package endorser

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracker"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type collectEndorsementsView struct {
	tx              *Transaction
	parties         []view.Identity
	deleteTransient bool
}

func (c *collectEndorsementsView) Call(context view.Context) (interface{}, error) {
	tracker, err := tracker.GetViewTracker(context)
	if err != nil {
		return nil, err
	}
	tracker.Report("collectEndorsementsView: Marshall State")

	signService := c.tx.FabricNetworkService().SigService()

	res, err := c.tx.Results()
	if err != nil {
		return nil, errors.Wrapf(err, "failed getting tx results")
	}

	for _, party := range c.parties {
		logger.Debugf("Collect Endorsements On Simulation from [%s]", party)

		if context.IsMe(party) {
			logger.Debugf("This is me %s, endorse locally.", party)
			// Endorse it
			err = c.tx.EndorseWithIdentity(party)
			if err != nil {
				return nil, errors.Wrap(err, "failed endorsing transaction")
			}
			continue
		}

		var txRaw []byte
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

		tracker.Report(fmt.Sprintf("collectEndorsementsView: collect signature from %s", party))
		session, err := context.GetSession(context.Initiator(), party)
		if err != nil {
			return nil, errors.Wrap(err, "failed getting session")
		}

		// Get a channel to receive the answer
		ch := session.Receive()

		// Send transaction
		err = session.Send(txRaw)
		if err != nil {
			return nil, errors.Wrap(err, "failed sending transaction content")
		}

		// Wait for the answer
		var msg *view.Message
		select {
		case msg = <-ch:
			tracker.Report(fmt.Sprintf("collectEndorsementsView: reply received from [%s]", party))
		case <-time.After(60 * time.Second):
			return nil, errors.Errorf("Timeout from party %s", party)
		}
		if msg.Status == view.ERROR {
			return nil, errors.New(string(msg.Payload))
		}

		// The response contains an array of marshalled ProposalResponse message
		var responses [][]byte
		if err := json.Unmarshal(msg.Payload, &responses); err != nil {
			return nil, errors.Wrapf(err, "failed unmarshalling response")
		}

		found := false
		tm := fabric.GetFabricNetworkService(context, c.tx.Network()).TransactionManager()
		for _, response := range responses {
			proposalResponse, err := tm.NewProposalResponseFromBytes(response)
			if err != nil {
				return nil, errors.Wrap(err, "failed unmarshalling received proposal response")
			}

			endorser := view.Identity(proposalResponse.Endorser())

			// Check the validity of the response
			if view2.GetEndpointService(context).IsBoundTo(endorser, party) {
				found = true
			}

			// Verify signatures
			verifier, err := signService.GetVerifier(endorser)
			if err != nil {
				return nil, errors.Wrapf(err, "failed getting verifier for party %s", party.String())
			}
			err = verifier.Verify(append(proposalResponse.Payload(), endorser...), proposalResponse.EndorserSignature())
			if err != nil {
				return nil, errors.Wrapf(err, "failed verifying endorsement for party %s", endorser.String())
			}
			// Check the content of the response
			// Now results can be equal to what this node has proposed or different
			if !bytes.Equal(res, proposalResponse.Results()) {
				return nil, errors.Errorf("received different results")
			}

			err = c.tx.AppendProposalResponse(proposalResponse)
			if err != nil {
				return nil, errors.Wrap(err, "failed appending received proposal response")
			}
		}

		if !found {
			return nil, errors.Errorf("invalid endorsement, expected one signed by [%s]", party.String())
		}

		tracker.Report(fmt.Sprintf("collectEndorsementsView: collected signature from %s", party))
	}
	tracker.Report(fmt.Sprintf("collectEndorsementsView done."))
	return c.tx, nil
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
		s.identities = []view.Identity{fabric.GetFabricNetworkService(context, s.tx.Network()).IdentityProvider().DefaultIdentity()}
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
	ch, err := fabric.GetDefaultNetwork(context).Channel(s.tx.Channel())
	if err != nil {
		return nil, errors.WithMessagef(err, "failed getting channel [%s]", s.tx.Channel())
	}
	err = ch.Vault().StoreTransaction(s.tx.ID(), txRaw)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed storing tx env [%s]", s.tx.ID())
	}

	// Send the proposal response back
	raw, err := json.Marshal(responses)
	if err != nil {
		return nil, err
	}

	err = context.Session().Send(raw)
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
