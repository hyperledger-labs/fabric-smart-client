/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ordering

import (
	"context"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/fabricutils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	common2 "github.com/hyperledger/fabric-protos-go/common"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
	context2 "golang.org/x/net/context"
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
	Name() string
	PickOrderer() *grpc.ConnectionConfig
	Orderers() []*grpc.ConnectionConfig
	LocalMembership() driver.LocalMembership
	// Broadcast sends the passed blob to the ordering Service to be ordered
	Broadcast(context context2.Context, blob interface{}) error
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

type TransactionWithEnvelope interface {
	Envelope() *common2.Envelope
}

type BroadcastFnc = func(context context.Context, env *common2.Envelope) error

type Service struct {
	SP      view2.ServiceProvider
	Network Network
	Metrics *metrics.Metrics

	Broadcasters   map[string]BroadcastFnc
	BroadcastMutex sync.RWMutex
	Broadcaster    BroadcastFnc
}

func NewService(sp view2.ServiceProvider, network Network, poolSize int, metrics *metrics.Metrics) *Service {
	s := &Service{
		SP:           sp,
		Network:      network,
		Metrics:      metrics,
		Broadcasters: map[string]BroadcastFnc{},
	}
	s.Broadcasters["BFT"] = NewBFTBroadcaster(network, poolSize, metrics).Broadcast
	cft := NewCFTBroadcaster(network, poolSize, metrics)
	s.Broadcasters["etcdraft"] = cft.Broadcast
	s.Broadcasters["solo"] = cft.Broadcast

	return s
}

func (o *Service) Broadcast(ctx context2.Context, blob interface{}) error {
	if ctx == nil {
		ctx = context.Background()
	}
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
	case TransactionWithEnvelope:
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

	o.BroadcastMutex.RLock()
	broadcaster := o.Broadcaster
	o.BroadcastMutex.RUnlock()
	if broadcaster == nil {
		return errors.Errorf("cannot broadcast yet, no consensus type set")
	}

	return broadcaster(ctx, env)
}

func (o *Service) SetConsensusType(consensusType string) error {
	logger.Debugf("ordering, setting consensus type to [%s]", consensusType)
	broadcaster, ok := o.Broadcasters[consensusType]
	if !ok {
		return errors.Errorf("no broadcaster found for consensus [%s]", consensusType)
	}
	o.BroadcastMutex.Lock()
	defer o.BroadcastMutex.Unlock()
	o.Broadcaster = broadcaster
	return nil
}

func (o *Service) createFabricEndorseTransactionEnvelope(tx Transaction) (*common2.Envelope, error) {
	ch, err := o.Network.Channel(tx.Channel())
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
	signer, err := o.Network.SignerService().GetSigner(signerID)
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
