/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ordering

import (
	"context"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/fabricutils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	common2 "github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

var logger = flogging.MustGetLogger("fabric-sdk.ordering")

type Signer interface {
	// Sign the message
	Sign(msg []byte) ([]byte, error)
}

type ViewManager interface {
	InitiateView(view view.View) (interface{}, error)
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
	env, err := fabricutils.CreateEndorserSignedTX(&signerWrapper{signerID, signer}, tx.Proposal(), tx.ProposalResponses()...)
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
	forceConnect := false
	stream, err := o.getOrSetOrdererClient()
	if err != nil {
		forceConnect = true
		logger.Errorf("failed to get ordering stream [%s]", err)
	}

	o.lock.Lock()
	defer o.lock.Unlock()

	// send the envelope for ordering
	var status *ab.BroadcastResponse
	retries := o.network.Config().BroadcastNumRetries()
	retryInterval := o.network.Config().BroadcastRetryInterval()
	for i := 0; i < retries; i++ {
		if i > 0 {
			logger.Debugf("broadcast, retry [%d]...", i)
			// wait a bit
			time.Sleep(retryInterval)
		}

		if i > 0 || forceConnect {
			o.cleanupOrderedClient()
			forceConnect = false
			// recreate client
			if err := o.newOrdererClient(); err != nil {
				logger.Errorf("failed to re-get ordering stream [%s], retry", err)
				continue
			}
			stream = o.oStream
		}

		err = BroadcastSend(stream, env)
		if err != nil {
			continue
		}

		status, err = stream.Recv()
		if err != nil {
			continue
		}

		if status.GetStatus() != common2.Status_SUCCESS {
			return errors.Wrapf(err, "failed broadcasting, status %s", common2.Status_name[int32(status.GetStatus())])
		}

		return nil
	}
	return errors.Wrap(err, "failed to send transaction to orderer")
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
