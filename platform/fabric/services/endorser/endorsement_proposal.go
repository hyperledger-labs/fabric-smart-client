/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package endorser

import (
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type parallelCollectEndorsementsOnProposalView struct {
	tx      *Transaction
	parties []view.Identity
}

type answer struct {
	payload []byte
	err     error
	party   view.Identity
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

	tm := fabric.GetFabricNetworkService(context, c.tx.Network()).TransactionManager()
	for i := 0; i < len(c.parties); i++ {
		// TODO: put a timeout
		answer := <-answerChannel
		if answer.err != nil {
			return nil, errors.Wrapf(answer.err, "got failure [%s] from [%s]", answer.party.String(), answer.err)
		}

		proposalResponse, err := tm.NewProposalResponseFromBytes(answer.payload)
		if err != nil {
			return nil, errors.Wrap(err, "failed unmarshalling received proposal response")
		}

		// TODO: check the validity of the response

		err = c.tx.AppendProposalResponse(proposalResponse)
		if err != nil {
			return nil, errors.Wrapf(answer.err, "failed appending response from [%s]", answer.party.String())
		}
	}
	return c.tx, nil
}

func (c *parallelCollectEndorsementsOnProposalView) callView(
	context view.Context,
	party view.Identity,
	raw []byte,
	answerChan chan *answer) {
	session, err := context.GetSession(context.Initiator(), party)
	if err != nil {
		answerChan <- &answer{err: err}
		return
	}

	// Wait to receive a Transaction back
	ch := session.Receive()

	err = session.Send(raw)
	if err != nil {
		answerChan <- &answer{err: err}
		return
	}
	msg := <-ch

	if msg.Status == view.ERROR {
		answerChan <- &answer{err: errors.New(string(msg.Payload))}
		return
	}

	answerChan <- &answer{payload: msg.Payload, party: party}
}

func NewParallelCollectEndorsementsOnProposalView(tx *Transaction, parties ...view.Identity) *parallelCollectEndorsementsOnProposalView {
	return &parallelCollectEndorsementsOnProposalView{tx: tx, parties: parties}
}

type endorsementsOnProposalResponderView struct {
	tx *Transaction
}

func (s *endorsementsOnProposalResponderView) Call(context view.Context) (interface{}, error) {
	err := s.tx.EndorseProposalResponseWithIdentity(context.Me())
	if err != nil {
		return nil, err
	}

	pr, err := s.tx.ProposalResponse()
	if err != nil {
		return nil, err
	}

	session := context.Session()
	if err != nil {
		return nil, err
	}

	// Send the proposal response back
	err = session.Send(pr)
	if err != nil {
		return nil, err
	}

	return s.tx, nil
}

func NewEndorsementOnProposalResponderView(tx *Transaction) *endorsementsOnProposalResponderView {
	return &endorsementsOnProposalResponderView{tx: tx}
}
