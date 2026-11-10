/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/protos"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/session"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/security"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type collectEndorsementsView struct {
	tx                *Transaction
	parties           []view.Identity
	deleteTransient   bool
	verifierProviders []fabric.VerifierProvider
}

func (c *collectEndorsementsView) Call(viewCtx view.Context) (any, error) {
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
	logger.DebugfContext(viewCtx.Context(), "Get results to send")
	res, err := c.tx.Results()
	if err != nil {
		return nil, errors.Wrap(err, "failed getting tx results")
	}

	// Contact sequentially all parties.
	logger.DebugfContext(viewCtx.Context(), "Collect Endorsements from [%d] parties [%v]", len(c.parties), c.parties)
	for _, party := range c.parties {
		logger.DebugfContext(viewCtx.Context(), "Collect Endorsements On Simulation from [%s]", party)

		var err error
		if viewCtx.IsMe(party) {
			logger.DebugfContext(viewCtx.Context(), "This is me %s, endorse locally.", party)
			// Endorse it
			err = c.tx.EndorseWithIdentity(party)
			if err != nil {
				return nil, errors.Wrap(err, "failed endorsing transaction")
			}
			continue
		}

		if err := RequireChannelKey(c.tx.Transaction); err != nil {
			return nil, err
		}
		var txRaw []byte
		logger.DebugfContext(viewCtx.Context(), "Get transaction bytes")
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

		logger.DebugfContext(viewCtx.Context(), "Get session with party")
		s, err := viewCtx.GetSession(viewCtx.Initiator(), party)
		if err != nil {
			return nil, errors.Wrap(err, "failed getting session")
		}
		txRaw, err = WireBytes(viewCtx.Context(), s, c.tx, txRaw)
		if err != nil {
			return nil, errors.WithMessage(err, "failed encoding transaction")
		}

		// Get a channel to receive the answer
		logger.DebugfContext(viewCtx.Context(), "Start receiving from party")
		ch := s.Receive()

		// Send transaction
		logger.DebugfContext(viewCtx.Context(), "Send raw transaction to party")
		err = s.Send(viewCtx.Context(), txRaw)
		if err != nil {
			return nil, errors.Wrap(err, "failed sending transaction content")
		}

		timeout := time.NewTimer(time.Minute)

		logger.DebugfContext(viewCtx.Context(), "Wait for transaction")
		// Wait for the answer
		var msg *view.Message
		select {
		case m, ok := <-ch:
			timeout.Stop()
			if !ok {
				return nil, &textError{msg: fmt.Sprintf("session to party %s closed", party), err: comm.ErrSessionClosed}
			}
			msg = m
		case <-viewCtx.Context().Done():
			timeout.Stop()
			return nil, errors.Wrapf(viewCtx.Context().Err(), "stopped waiting for party %s", party)
		case <-timeout.C:
			timeout.Stop()
			return nil, &textError{msg: fmt.Sprintf("Timeout from party %s", party), err: context.DeadlineExceeded}
		}
		logger.DebugfContext(viewCtx.Context(), "Received transaction from party")
		if msg.Status == view.ERROR {
			return nil, replyError(msg)
		}

		logger.Debugf("got response from party [%s]", party)

		responses, err := decodeResponses(msg.Payload)
		if err != nil {
			return nil, err
		}

		found := false
		fns, err := fabric.GetFabricNetworkService(viewCtx, c.tx.Network())
		if err != nil {
			return nil, errors.WithMessagef(err, "fabric network service [%s] not found", c.tx.Network())
		}
		tm := fns.TransactionManager()
		for _, response := range responses {
			logger.DebugfContext(viewCtx.Context(), "Parse proposal response")
			proposalResponse, err := tm.NewProposalResponseFromBytes(response)
			if err != nil {
				return nil, errors.Wrap(err, "failed unmarshalling received proposal response")
			}

			endorser := view.Identity(proposalResponse.Endorser())

			// Check the validity of the response
			logger.DebugfContext(viewCtx.Context(), "Check response validity")
			if endorser.Equal(party) || endpoint.GetService(viewCtx).IsBoundTo(viewCtx.Context(), endorser, party) {
				found = true
			}

			// TODO: check the verifier providers, if any
			verified := false
			logger.DebugfContext(viewCtx.Context(), "Start verification with providers")
			for _, provider := range vProviders {
				logger.DebugfContext(viewCtx.Context(), "Verify endorsement")
				err := proposalResponse.VerifyEndorsement(provider)
				if err == nil {
					logger.DebugfContext(viewCtx.Context(), "endorsement [%s] is valid", endorser)
					verified = true
					break
				}
				logger.DebugfContext(viewCtx.Context(), "endorsement [%s] is invalid, reason [%s]", endorser, err)
			}
			if !verified {
				return nil, errors.Errorf("failed to verify signature for party [%s][%s]", endorser.String(), string(endorser))
			}
			// Check the content of the response
			// Now results can be equal to what this node has proposed or different
			if !bytes.Equal(res, proposalResponse.Results()) {
				return nil, errors.New("received different results")
			}

			logger.DebugfContext(viewCtx.Context(), "append response from party [%s]", party)
			err = c.tx.AppendProposalResponse(proposalResponse)
			logger.DebugfContext(viewCtx.Context(), "Appended proposal response")
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

// replyError returns the error for msg, an error reply from a party. Its text is the payload.
// For a rejection, such as a draining or busy node, it wraps session.ErrRejected and the
// reason's sentinel, as session.RejectionError does; any other reply wraps nothing.
func replyError(msg *view.Message) error {
	return &textError{msg: string(msg.Payload), err: session.RejectionError(msg)}
}

// textError is an error whose text is msg and that wraps err, so that errors.Is matches err
// while the text stays msg.
type textError struct {
	msg string
	err error
}

func (e *textError) Error() string { return e.msg }

func (e *textError) Unwrap() error { return e.err }

// SetVerifierProviders sets additional providers used to verify the collected endorsements.
// They apply to the collection only; Transaction.HasBeenEndorsedBy consults the providers
// registered with Transaction.AppendVerifierProvider.
func (c *collectEndorsementsView) SetVerifierProviders(p []fabric.VerifierProvider) *collectEndorsementsView {
	c.verifierProviders = p
	return c
}

// NewCollectEndorsementsView returns a view that endorses tx with each of parties in turn:
// locally for an identity of this node, and otherwise by sending tx to the party and waiting
// up to a minute for its endorsement. It stops at the first party that fails, and a caller can
// match the error with errors.Is:
//   - session.ErrRejected and the rejection's sentinel, such as session.ErrDraining,
//     session.ErrBusy or session.ErrUnknownView, when the party rejected the session;
//   - comm.ErrSessionClosed when the session closed before the party answered;
//   - context.DeadlineExceeded when the party did not answer within the minute, and the
//     context's error when the view's context ended first;
//   - the send's error, such as a dial error, when tx could not be sent.
//
// Any other failure, such as an error reply that is not a rejection or an endorsement that
// does not verify, wraps none of these. Each endorsement is added to tx as soon as it
// verifies, and a failure does not remove those added before it; a party that fails before
// its reply verifies adds nothing.
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

func (s *endorseView) Call(viewCtx view.Context) (any, error) {
	if len(s.identities) == 0 {
		fns, err := fabric.GetFabricNetworkService(viewCtx, s.tx.Network())
		if err != nil {
			return nil, errors.WithMessagef(err, "fabric network service [%s] not found", s.tx.Network())
		}
		s.identities = []view.Identity{fns.IdentityProvider().DefaultIdentity()}
	}

	// The transaction may come from the remote peer; check it before signing or storing it.
	if err := checkReceived(viewCtx, sessionCaller(viewCtx), s.tx.Transaction); err != nil {
		return nil, err
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
	ch, err := s.tx.FabricNetworkService().Channel(s.tx.Channel())
	if err != nil {
		return nil, errors.WithMessagef(err, "failed getting channel [%s]", s.tx.Channel())
	}
	err = ch.Vault().StoreTransaction(viewCtx.Context(), s.tx.ID(), txRaw)
	if errors.Is(err, dbdriver.ErrConflict) {
		security.GetRejections(viewCtx).Report(viewCtx.Context(), security.KindStoreConflict, sessionCaller(viewCtx), s.tx.ID(), "a different transaction is stored under this txID")
	}
	if err != nil {
		return nil, errors.WithMessagef(err, "failed storing tx env [%s]", s.tx.ID())
	}

	// Send the proposal responses back, in the encoding the session negotiated.
	sess := viewCtx.Session()
	asProto, err := sendsEnvelope(viewCtx.Context(), sess, s.tx)
	if err != nil {
		return nil, err
	}
	var raw []byte
	if asProto {
		raw, err = proto.Marshal(&protos.EndorsementResponse{ProposalResponses: responses})
	} else {
		raw, err = json.Marshal(responses)
	}
	if err != nil {
		return nil, errors.Wrap(err, "failed marshalling endorsement response")
	}

	err = sess.Send(viewCtx.Context(), raw)
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
