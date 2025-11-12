/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/session"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// EndorsementsOnProposalTransaction models a transaction on which to collect endorsements on the transaction's proposal
type EndorsementsOnProposalTransaction interface {
	Network() string
	EndorseProposalResponseWithIdentity(id view.Identity) error
	ProposalResponses() ([][]byte, error)
	Bytes() ([]byte, error)
	ID() string
	AppendProposalResponse(response *fabric.ProposalResponse) error
}

type Response struct {
	ProposalResponses [][]byte
}

type answer struct {
	prs   [][]byte
	err   error
	party view.Identity
}

type parallelCollectEndorsementsOnProposalView struct {
	tx      EndorsementsOnProposalTransaction
	parties []view.Identity

	timeout time.Duration
}

func NewParallelCollectEndorsementsOnProposalView(tx *Transaction, parties ...view.Identity) *parallelCollectEndorsementsOnProposalView {
	return &parallelCollectEndorsementsOnProposalView{tx: tx, parties: parties}
}

func (c *parallelCollectEndorsementsOnProposalView) Call(ctx view.Context) (interface{}, error) {
	// send Transaction to each party and wait for their responses
	stateRaw, err := c.tx.Bytes()
	if err != nil {
		return nil, err
	}
	answerChannel := make(chan *answer, len(c.parties))

	logger.DebugfContext(ctx.Context(), "Collect endorsements from %d parties for TX [%s]", len(c.parties), c.tx.ID())
	for _, party := range c.parties {
		go c.collectEndorsement(ctx, party, stateRaw, answerChannel)
	}

	fns, err := fabric.GetFabricNetworkService(ctx, c.tx.Network())
	if err != nil {
		return nil, errors.WithMessagef(err, "fabric network service [%s] not found", c.tx.Network())
	}
	tm := fns.TransactionManager()
	for i := 0; i < len(c.parties); i++ {
		logger.DebugfContext(ctx.Context(), "Wait for endorsement")
		// TODO: put a timeout
		a := <-answerChannel
		logger.DebugfContext(ctx.Context(), "Received endorsement")
		if a.err != nil {
			return nil, errors.Wrapf(a.err, "got failure [%s] from [%s]", a.party.String(), a.err)
		}

		logger.Debugf("answer from [%s] contains [%d] responses, adding them", a.party, len(a.prs))

		for _, pr := range a.prs {
			logger.DebugfContext(ctx.Context(), "New proposal from bytes")
			proposalResponse, err := tm.NewProposalResponseFromBytes(pr)
			if err != nil {
				return nil, errors.Wrap(err, "failed unmarshalling received proposal response")
			}

			// TODO: check the validity of the response

			logger.DebugfContext(ctx.Context(), "Appended proposal")
			err = c.tx.AppendProposalResponse(proposalResponse)
			if err != nil {
				return nil, errors.Wrapf(a.err, "failed appending response from [%s]", a.party.String())
			}
		}
	}
	return c.tx, nil
}

func (c *parallelCollectEndorsementsOnProposalView) WithTimeout(timeout time.Duration) *parallelCollectEndorsementsOnProposalView {
	c.timeout = timeout
	return c
}

func (c *parallelCollectEndorsementsOnProposalView) collectEndorsement(
	context view.Context,
	party view.Identity,
	raw []byte,
	answerChan chan *answer) {
	defer logger.Debugf("Received answer for endorsement of TX [%s] from [%v]", c.tx.ID(), party)
	s, err := session.NewJSON(context, context.Initiator(), party)
	if err != nil {
		answerChan <- &answer{err: err, party: party}
		return
	}

	// Wait to receive a Transaction back
	logger.Debugf("Send transaction for TX [%s] signing to [%v]", c.tx.ID(), party)
	err = s.SendRaw(context.Context(), raw)
	logger.Debugf("Successfully sent transaction for TX [%s] signing to [%v]", c.tx.ID(), party)
	if err != nil {
		answerChan <- &answer{err: err, party: party}
		return
	}
	r := &Response{}
	if err := s.ReceiveWithTimeout(r, c.timeout); err != nil {
		answerChan <- &answer{err: err, party: party}
		return
	}
	answerChan <- &answer{prs: r.ProposalResponses, party: party}
}

type endorsementsOnProposalResponderView struct {
	tx         EndorsementsOnProposalTransaction
	identities []view.Identity
}

func NewEndorsementOnProposalResponderView(tx EndorsementsOnProposalTransaction, identities ...view.Identity) *endorsementsOnProposalResponderView {
	return &endorsementsOnProposalResponderView{tx: tx, identities: identities}
}

func (s *endorsementsOnProposalResponderView) Call(context view.Context) (interface{}, error) {
	if len(s.identities) == 0 {
		fns, err := fabric.GetFabricNetworkService(context, s.tx.Network())
		if err != nil {
			return nil, errors.WithMessagef(err, "fabric network service [%s] not found", s.tx.Network())
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

	session := session.JSON(context)
	if err != nil {
		return nil, err
	}

	// Send the proposal responses back
	err = session.SendWithContext(context.Context(), &Response{ProposalResponses: prs})
	if err != nil {
		return nil, err
	}
	return s.tx, nil
}
