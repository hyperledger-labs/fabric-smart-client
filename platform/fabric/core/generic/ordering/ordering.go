/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ordering

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	common2 "github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

var logger = flogging.MustGetLogger("fabric-sdk.ordering")

type Signer interface {
	// Sign the message
	Sign(msg []byte) ([]byte, error)
}

type SerializableSigner interface {
	Sign(message []byte) ([]byte, error)

	Serialize() ([]byte, error)
}

type ViewManager interface {
	InitiateView(view view.View) (interface{}, error)
}

type Configuration interface {
}

type Network interface {
	PickOrderer() *grpc.ConnectionConfig
	LocalMembership() driver.LocalMembership
	// Broadcast sends the passed blob to the ordering service to be ordered
	Broadcast(blob interface{}) error
	Channel(name string) (driver.Channel, error)
	SignerService() driver.SignerService
	Config() *config.Config
}

type Transaction interface {
	Channel() string
	ID() string
	Creator() view.Identity
	Proposal() driver.Proposal
	ProposalResponses() []driver.ProposalResponse
	Bytes() ([]byte, error)
}

type service struct {
	lock    sync.RWMutex
	oStream Broadcast
	oClient *ordererClient
	sp      view2.ServiceProvider
	network Network
}

func NewService(sp view2.ServiceProvider, network Network) *service {
	return &service{
		sp:      sp,
		network: network,
	}
}

func (o *service) Broadcast(blob interface{}) error {
	var env *common2.Envelope
	var err error
	switch b := blob.(type) {
	case Transaction:
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("new transaction to broadcast...")
		}
		env, err = o.createFabricEndorseTransactionEnvelope(b)
		if err != nil {
			return err
		}
	case *transaction.Envelope:
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("new envelope to broadcast (boxed)...")
		}
		env = b.Envelope()
	case *common2.Envelope:
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("new envelope to broadcast...")
		}
		env = blob.(*common2.Envelope)
	default:
		return errors.Errorf("invalid blob's type, got [%T]", blob)
	}

	return o.broadcastEnvelope(env)
}

func (o *service) createFabricEndorseTransactionEnvelope(tx Transaction) (*common2.Envelope, error) {
	ch, err := o.network.Channel(tx.Channel())
	if err != nil {
		return nil, errors.Wrapf(err, "failed getting channel [%s]", tx.Channel())
	}
	txRaw, err := tx.Bytes()
	if err != nil {
		return nil, errors.Wrapf(err, "failed marshalling tx [%s]", tx.ID())
	}
	err = ch.TransactionService().StoreTransaction(tx.ID(), txRaw)
	if err != nil {
		return nil, errors.Wrap(err, "failed storing tx")
	}

	// tx contains the proposal and the endorsements, assemble them in a fabric transaction
	signerID := tx.Creator()
	signer, err := o.network.SignerService().GetSigner(signerID)
	if err != nil {
		logger.Errorf("signer not found for %s while creating tx envelope for ordering [%s]", signerID.UniqueID(), err)
		return nil, errors.Wrapf(err, "signer not found for %s while creating tx envelope for ordering", signerID.UniqueID())
	}
	env, err := createSignedTx(
		tx.Proposal(),
		&signerWrapper{signerID, signer},
		tx.ProposalResponses()...,
	)
	if err != nil {
		return nil, errors.WithMessage(err, "could not assemble transaction")
	}

	return env, nil
}

func (o *service) getOrSetOrdererClient() (Broadcast, error) {
	o.lock.RLock()
	oc := o.oClient
	os := o.oStream
	o.lock.RUnlock()

	if oc != nil {
		return os, nil
	}

	o.lock.Lock()
	defer o.lock.Unlock()

	if o.oClient != nil {
		return o.oStream, nil
	}

	if err := o.newOrdererClient(); err != nil {
		return nil, err
	}

	return o.oStream, nil
}

func (o *service) newOrdererClient() error {
	ordererConfig := o.network.PickOrderer()
	if ordererConfig == nil {
		return errors.New("no orderer configured")
	}

	oClient, err := NewOrdererClient(ordererConfig)
	if err != nil {
		return errors.Wrapf(err, "failed creating orderer client for %s", ordererConfig.Address)
	}

	stream, err := oClient.NewBroadcast(context.Background())
	if err != nil {
		oClient.Close()
		return errors.Wrapf(err, "failed creating orderer stream for %s", ordererConfig.Address)
	}

	o.oStream = stream
	o.oClient = oClient

	return nil
}

func (o *service) cleanupOrderedClient() {
	if o.oStream != nil {
		logger.Debugf("cleanup ordering stream...")
		o.oStream.CloseSend()
		o.oStream = nil
	}
	if o.oClient != nil {
		logger.Debugf("cleanup ordering client to [%s]", o.oClient.ordererAddr)
		o.oClient.Close()
		o.oClient = nil
	}
}

func (o *service) broadcastEnvelope(env *common2.Envelope) error {
	stream, err := o.getOrSetOrdererClient()
	if err != nil {
		return errors.WithMessagef(err, "failed getting ordering stream")
	}

	o.lock.Lock()
	defer o.lock.Unlock()

	// send the envelope for ordering
	var status *ab.BroadcastResponse
	retries := o.network.Config().BroadcastNumRetries()
	retryInternal := o.network.Config().BroadcastRetryInterval()
	for i := 0; i < retries; i++ {
		if i > 0 {
			logger.Debugf("broadcast, retry [%d]...", i)
			// wait a bit
			time.Sleep(retryInternal)
			// recreate client
			if err := o.newOrdererClient(); err != nil {
				return errors.WithMessagef(err, "failed re-getting ordering stream")
			}
			stream = o.oStream
		}

		err = BroadcastSend(stream, env)
		if err != nil {
			o.cleanupOrderedClient()
			continue
		}

		status, err = stream.Recv()
		if err != nil {
			o.cleanupOrderedClient()
			continue
		}

		if status.GetStatus() != common2.Status_SUCCESS {
			return errors.Wrapf(err, "failed broadcasting, status %s", common2.Status_name[int32(status.GetStatus())])
		}

		return nil
	}
	return errors.Wrap(err, "failed to send transaction to orderer")
}

// createSignedTx assembles an Envelope message from proposal, endorsements,
// and a signer. This function should be called by a client when it has
// collected enough endorsements for a proposal to create a transaction and
// submit it to peers for ordering
func createSignedTx(proposal driver.Proposal, signer SerializableSigner, resps ...driver.ProposalResponse) (*common2.Envelope, error) {
	if len(resps) == 0 {
		return nil, errors.New("at least one proposal response is required")
	}

	// the original header
	hdr, err := protoutil.UnmarshalHeader(proposal.Header())
	if err != nil {
		return nil, err
	}

	// the original payload
	pPayl, err := protoutil.UnmarshalChaincodeProposalPayload(proposal.Payload())
	if err != nil {
		return nil, err
	}

	// check that the signer is the same that is referenced in the header
	// TODO: maybe worth removing?
	signerBytes, err := signer.Serialize()
	if err != nil {
		return nil, err
	}

	shdr, err := protoutil.UnmarshalSignatureHeader(hdr.SignatureHeader)
	if err != nil {
		return nil, err
	}

	if !bytes.Equal(signerBytes, shdr.Creator) {
		return nil, errors.New("signer must be the same as the one referenced in the header")
	}

	// ensure that all actions are bitwise equal and that they are successful
	var a1 []byte
	var first driver.ProposalResponse
	for n, r := range resps {
		if r.ResponseStatus() < 200 || r.ResponseStatus() >= 400 {
			return nil, errors.Errorf("proposal response was not successful, error code %d, msg %s", r.ResponseStatus(), r.ResponseMessage())
		}

		if n == 0 {
			a1 = r.Payload()
			first = r
			continue
		}

		if !bytes.Equal(a1, r.Payload()) {
			upr1, err := UnpackProposalResponse(first.Payload())
			if err != nil {
				return nil, err
			}
			rwset1, err := json.MarshalIndent(upr1.TxRwSet, "", "  ")
			if err != nil {
				return nil, err
			}

			upr2, err := UnpackProposalResponse(r.Payload())
			if err != nil {
				return nil, err
			}
			rwset2, err := json.MarshalIndent(upr2.TxRwSet, "", "  ")
			if err != nil {
				return nil, err
			}

			if !bytes.Equal(rwset1, rwset2) {
				if logger.IsEnabledFor(zapcore.DebugLevel) {
					logger.Debugf("ProposalResponsePayloads do not match (%v) \n[%s]\n!=\n[%s]",
						bytes.Equal(rwset1, rwset2), string(rwset1), string(rwset2),
					)
				}
			} else {
				pr1, err := json.MarshalIndent(first, "", "  ")
				if err != nil {
					return nil, err
				}
				pr2, err := json.MarshalIndent(r, "", "  ")
				if err != nil {
					return nil, err
				}

				if logger.IsEnabledFor(zapcore.DebugLevel) {
					logger.Debugf("ProposalResponse do not match  \n[%s]\n!=\n[%s]",
						bytes.Equal(pr1, pr2), string(pr1), string(pr2),
					)
				}
			}

			return nil, errors.Errorf(
				"ProposalResponsePayloads do not match [%s]!=[%s]",
				base64.StdEncoding.EncodeToString(a1),
				base64.StdEncoding.EncodeToString(r.Payload()),
			)
		}
	}

	// fill endorsements
	endorsements := make([]*peer.Endorsement, len(resps))
	for n, r := range resps {
		endorsements[n] = &peer.Endorsement{
			Endorser:  r.Endorser(),
			Signature: r.EndorserSignature(),
		}
	}

	// create ChaincodeEndorsedAction
	cea := &peer.ChaincodeEndorsedAction{ProposalResponsePayload: resps[0].Payload(), Endorsements: endorsements}

	// obtain the bytes of the proposal payload that will go to the transaction
	propPayloadBytes, err := protoutil.GetBytesProposalPayloadForTx(pPayl)
	if err != nil {
		return nil, err
	}

	// serialize the chaincode action payload
	cap := &peer.ChaincodeActionPayload{ChaincodeProposalPayload: propPayloadBytes, Action: cea}
	capBytes, err := protoutil.GetBytesChaincodeActionPayload(cap)
	if err != nil {
		return nil, err
	}

	// create a transaction
	taa := &peer.TransactionAction{Header: hdr.SignatureHeader, Payload: capBytes}
	taas := make([]*peer.TransactionAction, 1)
	taas[0] = taa
	tx := &peer.Transaction{Actions: taas}

	// serialize the tx
	txBytes, err := protoutil.GetBytesTransaction(tx)
	if err != nil {
		return nil, err
	}

	// create the payload
	payl := &common2.Payload{Header: hdr, Data: txBytes}
	paylBytes, err := protoutil.GetBytesPayload(payl)
	if err != nil {
		return nil, err
	}

	// sign the payload
	sig, err := signer.Sign(paylBytes)
	if err != nil {
		return nil, err
	}

	// here's the envelope
	return &common2.Envelope{Payload: paylBytes, Signature: sig}, nil
}

type signerWrapper struct {
	creator view.Identity
	signer  Signer
}

func (s *signerWrapper) Sign(message []byte) ([]byte, error) {
	return s.signer.Sign(message)
}

func (s *signerWrapper) Serialize() ([]byte, error) {
	return s.creator, nil
}
