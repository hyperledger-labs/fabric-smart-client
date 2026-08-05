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
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// EndorsementsOnProposalTransaction models a transaction on which to collect endorsements on the transaction's proposal
type EndorsementsOnProposalTransaction interface {
	Network() string
	Channel() string
	EndorseProposalResponseWithIdentity(id view.Identity) error
	ProposalResponses() ([][]byte, error)
	Bytes() ([]byte, error)
	ID() string
	AppendProposalResponse(response *fabric.ProposalResponse) error
	FabricNetworkService() *fabric.NetworkService
}

type Response struct {
	ProposalResponses [][]byte
}

type answer struct {
	prs   [][]byte
	err   error
	party view.Identity
}

// defaultParallelEndorsementTimeout is the timeout applied when none has been configured via
// WithTimeout. It matters beyond just the final receive step: session establishment and
// sending the proposal to each party (both done inside collectEndorsement, before its own
// receive-timeout kicks in) are not otherwise bounded, so an unresponsive or malicious party
// could park its goroutine indefinitely and, with it, this view's wait on answerChannel.
// See parallelEndorsementDeadlineGrace for how the timeout maps onto the two deadlines.
const defaultParallelEndorsementTimeout = 30 * time.Second

// parallelEndorsementDeadlineGrace is how much longer the aggregate deadline runs than the
// per-party receive timeout that collectEndorsement arms with the configured timeout. Without
// it the two expire together and the select in Call picks between them at random, so a silent
// party is reported as often by the generic aggregate error as by the per-party one that names
// it. The grace lets the per-party timeout land first, leaving the aggregate deadline as what
// it is meant to be: the backstop for the session setup and send steps, which nothing else
// bounds.
//
// The practical consequence is that the configured timeout bounds each party individually,
// and the initiator gives up after timeout+grace overall rather than at timeout exactly. The
// grace is a fixed amount, not a fraction, so it is negligible next to the default timeout but
// dominates a caller-configured one in the tens of milliseconds.
const parallelEndorsementDeadlineGrace = 500 * time.Millisecond

type parallelCollectEndorsementsOnProposalView struct {
	tx      EndorsementsOnProposalTransaction
	parties []view.Identity

	timeout time.Duration
}

func NewParallelCollectEndorsementsOnProposalView(tx *Transaction, parties ...view.Identity) *parallelCollectEndorsementsOnProposalView {
	return &parallelCollectEndorsementsOnProposalView{tx: tx, parties: parties}
}

func (c *parallelCollectEndorsementsOnProposalView) Call(viewCtx view.Context) (any, error) {
	// send Transaction to each party and wait for their responses
	stateRaw, err := c.tx.Bytes()
	if err != nil {
		return nil, err
	}
	answerChannel := make(chan *answer, len(c.parties))

	timeout := c.timeout
	if timeout <= 0 {
		timeout = defaultParallelEndorsementTimeout
	}

	logger.DebugfContext(viewCtx.Context(), "Collect endorsements from %d parties for TX [%s]", len(c.parties), c.tx.ID())
	for _, party := range c.parties {
		go c.collectEndorsement(viewCtx, party, stateRaw, timeout, answerChannel)
	}

	fns, err := fabric.GetFabricNetworkService(viewCtx, c.tx.Network())
	if err != nil {
		return nil, errors.WithMessagef(err, "fabric network service [%s] not found", c.tx.Network())
	}
	tm := fns.TransactionManager()

	ch, err := fns.Channel(c.tx.Channel())
	if err != nil {
		return nil, errors.Wrapf(err, "failed getting channel [%s:%s]", c.tx.Network(), c.tx.Channel())
	}
	vProviders := []fabric.VerifierProvider{&verifierProviderWrapper{m: ch.MSPManager()}}

	deadline := time.NewTimer(timeout + parallelEndorsementDeadlineGrace)
	defer deadline.Stop()

	for i := 0; i < len(c.parties); i++ {
		logger.DebugfContext(viewCtx.Context(), "Wait for endorsement")
		var a *answer
		select {
		case a = <-answerChannel:
		case <-deadline.C:
			return nil, errors.Errorf("timeout waiting for endorsement from [%d] parties", len(c.parties)-i)
		}
		logger.DebugfContext(viewCtx.Context(), "Received endorsement")
		if a.err != nil {
			return nil, errors.Wrapf(a.err, "got failure from [%s]", a.party.String())
		}

		logger.Debugf("answer from [%s] contains [%d] responses, adding them", a.party, len(a.prs))

		for _, pr := range a.prs {
			logger.DebugfContext(viewCtx.Context(), "New proposal from bytes")
			proposalResponse, err := tm.NewProposalResponseFromBytes(pr)
			if err != nil {
				return nil, errors.Wrap(err, "failed unmarshalling received proposal response")
			}

			endorserID := view.Identity(proposalResponse.Endorser())
			if !endorserID.Equal(a.party) && !endpoint.GetService(viewCtx).IsBoundTo(viewCtx.Context(), endorserID, a.party) {
				return nil, errors.Errorf("invalid endorsement, expected one signed by [%s]", a.party.String())
			}

			verified := false
			for _, provider := range vProviders {
				if err := proposalResponse.VerifyEndorsement(provider); err == nil {
					verified = true
					break
				}
			}
			if !verified {
				return nil, errors.Errorf("failed to verify signature for party [%s]", a.party.String())
			}

			logger.DebugfContext(viewCtx.Context(), "Appended proposal")
			err = c.tx.AppendProposalResponse(proposalResponse)
			if err != nil {
				return nil, errors.Wrapf(err, "failed appending response from [%s]", a.party.String())
			}
		}
	}
	return c.tx, nil
}

// WithTimeout sets how long each contacted party has to answer. Call gives up on the
// collection as a whole after timeout+parallelEndorsementDeadlineGrace, so that a party that
// answers nothing is reported by name rather than by the generic aggregate error; see
// parallelEndorsementDeadlineGrace. A timeout of zero or less selects
// defaultParallelEndorsementTimeout.
func (c *parallelCollectEndorsementsOnProposalView) WithTimeout(timeout time.Duration) *parallelCollectEndorsementsOnProposalView {
	c.timeout = timeout
	return c
}

func (c *parallelCollectEndorsementsOnProposalView) collectEndorsement(
	viewCtx view.Context,
	party view.Identity,
	raw []byte,
	timeout time.Duration,
	answerChan chan *answer,
) {
	defer logger.Debugf("Received answer for endorsement of TX [%s] from [%v]", c.tx.ID(), party)
	s, err := session.NewJSON(viewCtx, viewCtx.Initiator(), party)
	if err != nil {
		answerChan <- &answer{err: err, party: party}
		return
	}

	// Wait to receive a Transaction back
	logger.Debugf("Send transaction for TX [%s] signing to [%v]", c.tx.ID(), party)
	err = s.SendRaw(viewCtx.Context(), raw)
	logger.Debugf("Successfully sent transaction for TX [%s] signing to [%v]", c.tx.ID(), party)
	if err != nil {
		answerChan <- &answer{err: err, party: party}
		return
	}
	r := &Response{}
	if err := s.ReceiveWithTimeout(r, timeout); err != nil {
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

func (s *endorsementsOnProposalResponderView) Call(viewCtx view.Context) (any, error) {
	if len(s.identities) == 0 {
		fns, err := fabric.GetFabricNetworkService(viewCtx, s.tx.Network())
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

	session := session.JSON(viewCtx)
	if err != nil {
		return nil, err
	}

	// Send the proposal responses back
	err = session.SendWithContext(viewCtx.Context(), &Response{ProposalResponses: prs})
	if err != nil {
		return nil, err
	}
	return s.tx, nil
}
