/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ordering

import (
	"context"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	common2 "github.com/hyperledger/fabric-protos-go/common"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
	context2 "golang.org/x/net/context"
)

type ConsensusType = string

const (
	BFT  ConsensusType = "BFT"
	Raft ConsensusType = "etcdraft"
	Solo ConsensusType = "solo"
)

var logger = logging.MustGetLogger("fabric-sdk.ordering")

type Transaction interface {
	Channel() string
	ID() string
	Creator() view.Identity
	Proposal() driver.Proposal
	ProposalResponses() ([]driver.ProposalResponse, error)
	Bytes() ([]byte, error)
	Envelope() (driver.Envelope, error)
}

type TransactionWithEnvelope interface {
	Envelope() *common2.Envelope
}

type BroadcastFnc = func(context context.Context, env *common2.Envelope) error

type GetEndorserTransactionServiceFunc = func(channelID string) (driver.EndorserTransactionService, error)

type Service struct {
	GetEndorserTransactionService GetEndorserTransactionServiceFunc
	SigService                    driver.SignerService
	Metrics                       *metrics.Metrics

	Broadcasters   map[ConsensusType]BroadcastFnc
	BroadcastMutex sync.RWMutex
	Broadcaster    BroadcastFnc
}

func NewService(
	getEndorserTransactionService GetEndorserTransactionServiceFunc,
	sigService driver.SignerService,
	configService driver.ConfigService,
	metrics *metrics.Metrics,
	services Services,
) *Service {
	s := &Service{
		GetEndorserTransactionService: getEndorserTransactionService,
		SigService:                    sigService,
		Metrics:                       metrics,
		Broadcasters:                  map[ConsensusType]BroadcastFnc{},
		BroadcastMutex:                sync.RWMutex{},
		Broadcaster:                   nil,
	}
	s.Broadcasters[BFT] = NewBFTBroadcaster(configService, services, metrics).Broadcast
	cft := NewCFTBroadcaster(configService, services, metrics)
	s.Broadcasters[Raft] = cft.Broadcast
	s.Broadcasters[Solo] = cft.Broadcast

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

func (o *Service) SetConsensusType(consensusType ConsensusType) error {
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
	env, err := tx.Envelope()
	if err != nil {
		return nil, errors.Wrapf(err, "failed creating envelope for transaction [%s]", tx.ID())
	}
	raw, err := env.Bytes()
	if err != nil {
		return nil, errors.Wrapf(err, "failed marshalling envelope for transaction [%s]", tx.ID())
	}
	commonEnv := &common2.Envelope{}
	if err := proto.Unmarshal(raw, commonEnv); err != nil {
		return nil, errors.Wrapf(err, "failed unmarshalling envelope for transaction [%s]", tx.ID())
	}
	return commonEnv, nil
}
