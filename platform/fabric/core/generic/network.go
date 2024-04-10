/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/ordering"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/rwset"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("fabric-sdk.core")

type NewChannelFunc = func(network driver.FabricNetworkService, name string, quiet bool) (driver.Channel, error)

type Network struct {
	SP   view2.ServiceProvider
	name string

	configService      driver.ConfigService
	localMembership    driver.LocalMembership
	idProvider         driver.IdentityProvider
	processorManager   driver.ProcessorManager
	transactionManager driver.TransactionManager
	sigService         driver.SignerService

	ConsensusType string
	Ordering      driver.Ordering

	Metrics *metrics.Metrics

	NewChannel   NewChannelFunc
	ChannelMap   map[string]driver.Channel
	ChannelMutex sync.RWMutex
}

func NewNetwork(
	sp view2.ServiceProvider,
	name string,
	config driver.ConfigService,
	idProvider driver.IdentityProvider,
	localMembership driver.LocalMembership,
	sigService driver.SignerService,
	metrics *metrics.Metrics,
	newChannel NewChannelFunc,
) (*Network, error) {
	return &Network{
		SP:              sp,
		name:            name,
		configService:   config,
		ChannelMap:      map[string]driver.Channel{},
		localMembership: localMembership,
		idProvider:      idProvider,
		sigService:      sigService,
		Metrics:         metrics,
		NewChannel:      newChannel,
	}, nil
}

func (f *Network) Name() string {
	return f.name
}

func (f *Network) Channel(name string) (driver.Channel, error) {
	logger.Debugf("Getting Channel [%s]", name)

	if len(name) == 0 {
		name = f.ConfigService().DefaultChannel()
		logger.Debugf("Resorting to default Channel [%s]", name)
	}

	chanQuiet := f.ConfigService().IsChannelQuite(name)

	// first check the cache
	f.ChannelMutex.RLock()
	ch, ok := f.ChannelMap[name]
	f.ChannelMutex.RUnlock()
	if ok {
		logger.Debugf("Returning Channel for [%s]", name)
		return ch, nil
	}

	// create Channel and store in cache
	f.ChannelMutex.Lock()
	defer f.ChannelMutex.Unlock()

	ch, ok = f.ChannelMap[name]
	if !ok {
		logger.Debugf("Channel [%s] not found, allocate resources", name)
		var err error
		ch, err = f.NewChannel(f, name, chanQuiet)
		if err != nil {
			return nil, err
		}
		f.ChannelMap[name] = ch
		logger.Debugf("Channel [%s] not found, created", name)
	}

	logger.Debugf("Returning Channel for [%s]", name)
	return ch, nil
}

func (f *Network) Ledger(name string) (driver.Ledger, error) {
	return f.Channel(name)
}

func (f *Network) Committer(name string) (driver.Committer, error) {
	return f.Channel(name)
}

func (f *Network) IdentityProvider() driver.IdentityProvider {
	return f.idProvider
}

func (f *Network) LocalMembership() driver.LocalMembership {
	return f.localMembership
}

func (f *Network) ProcessorManager() driver.ProcessorManager {
	return f.processorManager
}

func (f *Network) TransactionManager() driver.TransactionManager {
	return f.transactionManager
}

func (f *Network) OrderingService() driver.Ordering {
	return f.Ordering
}

func (f *Network) SignerService() driver.SignerService {
	return f.sigService
}

func (f *Network) ConfigService() driver.ConfigService {
	return f.configService
}

func (f *Network) Init() error {
	f.processorManager = rwset.NewProcessorManager(f, nil)
	f.transactionManager = transaction.NewManager()
	f.transactionManager.AddTransactionFactory(
		driver.EndorserTransaction,
		transaction.NewEndorserTransactionFactory(f.Name(), f, f.sigService),
	)
	f.Ordering = ordering.NewService(
		func(channelID string) (driver.EndorserTransactionService, error) {
			ch, err := f.Channel(channelID)
			if err != nil {
				return nil, err
			}
			return ch.TransactionService(), nil
		},
		f.sigService,
		f.configService,
		f.Metrics,
	)
	return nil
}

func (f *Network) SetConfigOrderers(o channelconfig.Orderer, orderers []*grpc.ConnectionConfig) error {
	if err := f.Ordering.SetConsensusType(o.ConsensusType()); err != nil {
		return errors.WithMessagef(err, "failed to set consensus type from channel config")
	}
	if err := f.ConfigService().SetConfigOrderers(orderers); err != nil {
		return errors.WithMessagef(err, "failed to set ordererss")
	}
	return nil
}

func (f *Network) SetTransactionManager(tm driver.TransactionManager) {
	f.transactionManager = tm
}

func (f *Network) SetProcessorManager(pm driver.ProcessorManager) {
	f.processorManager = pm
}
