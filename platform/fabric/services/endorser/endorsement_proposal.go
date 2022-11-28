/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	session2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/session"
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type parallelCollectEndorsementsOnProposalView struct {
	tx      *Transaction
	parties []view.Identity
}

type response struct {
	prs [][]byte
}

type answer struct {
	prs   [][]byte
	err   error
	party view.Identity
}

func (c *parallelCollectEndorsementsOnProposalView) Call(context view.Context) (interface{}, error) {
	// send Transaction to each party and wait for their responses
	stateRaw, err := c.tx.Bytes()
	if err != nil {
		return nil, err
	}
	answerChannel := make(chan *answer, len(c.parties))
	for _, party := range c.parties {
		go c.callView(context, party, stateRaw, answerChannel)
	}

	fns := fabric.GetFabricNetworkService(context, c.tx.Network())
	if fns == nil {
		return nil, errors.Errorf("fabric network service [%s] not found", c.tx.Network())
	}
	tm := fns.TransactionManager()
	for i := 0; i < len(c.parties); i++ {
		// TODO: put a timeout
		a := <-answerChannel
		if a.err != nil {
			return nil, errors.Wrapf(a.err, "got failure [%s] from [%s]", a.party.String(), a.err)
		}

		logger.Debugf("answer from [%s] contains [%d] responses, adding them", a.party, len(a.prs))

		for _, pr := range a.prs {
			proposalResponse, err := tm.NewProposalResponseFromBytes(pr)
			if err != nil {
				return nil, errors.Wrap(err, "failed unmarshalling received proposal response")
			}

			// TODO: check the validity of the response

			err = c.tx.AppendProposalResponse(proposalResponse)
			if err != nil {
				return nil, errors.Wrapf(a.err, "failed appending response from [%s]", a.party.String())
			}
		}
	}
	return c.tx, nil
}

func (c *parallelCollectEndorsementsOnProposalView) callView(
	context view.Context,
	party view.Identity,
	raw []byte,
	answerChan chan *answer) {

	session, err := session2.NewJSON(context, context.Initiator(), party)
	if err != nil {
		answerChan <- &answer{err: err}
		return
	}

	// Wait to receive a Transaction back
	err = session.SendRaw(raw)
	if err != nil {
		answerChan <- &answer{err: err}
		return
	}
	r := &response{}
	if err := session.Receive(r); err != nil {
		answerChan <- &answer{err: err}
		return
	}
	answerChan <- &answer{prs: r.prs, party: party}
}

func NewParallelCollectEndorsementsOnProposalView(tx *Transaction, parties ...view.Identity) *parallelCollectEndorsementsOnProposalView {
	return &parallelCollectEndorsementsOnProposalView{tx: tx, parties: parties}
}

type endorsementsOnProposalResponderView struct {
	tx         *Transaction
	identities []view.Identity
}

func (s *endorsementsOnProposalResponderView) Call(context view.Context) (interface{}, error) {
	if len(s.identities) == 0 {
		fns := fabric.GetFabricNetworkService(context, s.tx.Network())
		if fns == nil {
			return nil, errors.Errorf("fabric network service [%s] not found", s.tx.Network())
		}
		s.identities = []view.Identity{fns.IdentityProvider().DefaultIdentity()}
	}

	for _, id := range s.identities {
		logger.Debugf("endorse proposal response with [%s]", id)
		err := s.tx.EndorseProposalResponseWithIdentity(id)
		if err != nil {
			return nil, err
		}
	}

	prs, err := s.tx.ProposalResponses()
	if err != nil {
		return nil, err
	}
	logger.Debugf("number of endorse proposal response produced [%d], send them back", len(prs))

	session := session2.JSON(context)
	if err != nil {
		return nil, err
	}

	// Send the proposal responses back
	err = session.Send(&response{prs: prs})
	if err != nil {
		return nil, err
	}
	return s.tx, nil
}

func NewEndorsementOnProposalResponderView(tx *Transaction, identities ...view.Identity) *endorsementsOnProposalResponderView {
	return &endorsementsOnProposalResponderView{tx: tx, identities: identities}
}
