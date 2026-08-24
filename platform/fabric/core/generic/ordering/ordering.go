/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ordering

import (
	"context"

	common2 "github.com/hyperledger/fabric-protos-go-apiv2/common"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/configstate"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type ConsensusType = string

const (
	BFT  ConsensusType = "BFT"
	Raft ConsensusType = "etcdraft"
	Solo ConsensusType = "solo"
)

var logger = logging.MustGetLogger()

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

type BroadcastFnc = func(ctx context.Context, env *common2.Envelope) error

type GetEndorserTransactionServiceFunc = func(channelID string) (driver.EndorserTransactionService, error)

// Service broadcasts envelopes to the ordering service of a network using the
// broadcaster for the consensus type in force.
//
// Which consensus type that is comes from the channel configuration, which is
// loaded asynchronously after the service is built: the committer feeds it to
// MembershipService.Update, reads it back with OrdererConfig, and passes it here
// via Configure. Until that happens the service has no broadcaster and Broadcast
// reports driver.ErrNotInitialized.
type Service struct {
	GetEndorserTransactionService GetEndorserTransactionServiceFunc
	SigService                    driver.SignerService
	ConfigService                 driver.ConfigService
	Metrics                       *metrics.Metrics

	// Broadcasters holds one broadcaster per supported consensus type. It is
	// populated by NewService and not written afterwards.
	Broadcasters map[ConsensusType]BroadcastFnc

	// broadcaster holds the broadcaster selected by SetConsensusType. It is
	// unexported and reached only through configstate.Holder so that the
	// "always read under the lock" rule cannot be sidestepped, and so that the
	// not-yet-selected case reaches the caller as an error rather than as a nil
	// function value.
	broadcaster *configstate.Holder[BroadcastFnc]
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
		broadcaster:                   configstate.NewHolder[BroadcastFnc]("network [" + configService.NetworkName() + "] ordering configuration"),
		ConfigService:                 configService,
	}
	s.Broadcasters[BFT] = NewBFTBroadcaster(configService, services, metrics).Broadcast
	cft := NewCFTBroadcaster(configService, services, metrics)
	s.Broadcasters[Raft] = cft.Broadcast
	s.Broadcasters[Solo] = cft.Broadcast

	return s
}

func (o *Service) Broadcast(ctx context.Context, blob any) error {
	if ctx == nil {
		ctx = context.Background()
	}
	defer logger.DebugfContext(ctx, "Done broadcasting")
	var env *common2.Envelope
	var err error
	switch b := blob.(type) {
	case Transaction:
		logger.DebugfContext(ctx, "new transaction to broadcast...")
		env, err = o.createFabricEndorseTransactionEnvelope(b)
		if err != nil {
			return err
		}
	case TransactionWithEnvelope:
		logger.DebugfContext(ctx, "new envelope to broadcast (boxed)...")
		env = b.Envelope()
	case *common2.Envelope:
		logger.DebugfContext(ctx, "new envelope to broadcast...")
		env = blob.(*common2.Envelope)
	default:
		logger.ErrorfContext(ctx, "invalid blob type [%T]", blob)
		return errors.Errorf("invalid blob's type, got [%T]", blob)
	}

	logger.DebugfContext(ctx, "Acquire broadcaster")
	broadcaster, err := o.broadcaster.Get()
	if err != nil {
		return errors.WithMessage(err, "cannot broadcast yet, no consensus type set")
	}

	logger.DebugfContext(ctx, "Broadcast")
	return broadcaster(ctx, env)
}

// SetConsensusType selects the broadcaster for consensusType. An unsupported
// consensus type leaves any previously selected broadcaster in place.
func (o *Service) SetConsensusType(consensusType ConsensusType) error {
	logger.Debugf("ordering, setting consensus type to [%s]", consensusType)

	return o.broadcaster.Update(func(BroadcastFnc, bool) (BroadcastFnc, error) {
		broadcaster, ok := o.Broadcasters[consensusType]
		if !ok {
			return nil, errors.Errorf("no broadcaster found for consensus [%s]", consensusType)
		}
		return broadcaster, nil
	})
}

func (f *Service) Configure(consensusType string, orderers []*grpc.ConnectionConfig) error {
	if err := f.SetConsensusType(consensusType); err != nil {
		return errors.WithMessagef(err, "failed to set consensus type from channel config")
	}
	if err := f.ConfigService.SetConfigOrderers(orderers); err != nil {
		return errors.WithMessagef(err, "failed to set ordererss")
	}
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
