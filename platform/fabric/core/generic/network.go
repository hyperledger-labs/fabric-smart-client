/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"math/rand"
	"sync"

	"golang.org/x/net/context"

	config2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/ordering"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/rwset"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("fabric-sdk.core")

type NewChannelFunc = func(network driver.FabricNetworkService, name string, quiet bool) (driver.Channel, error)

type Network struct {
	SP   view2.ServiceProvider
	name string

	config *config2.Config

	localMembership    driver.LocalMembership
	idProvider         driver.IdentityProvider
	processorManager   driver.ProcessorManager
	transactionManager driver.TransactionManager
	sigService         driver.SignerService

	orderers           []*grpc.ConnectionConfig
	configuredOrderers int
	peers              map[driver.PeerFunctionType][]*grpc.ConnectionConfig
	defaultChannel     string
	channelConfigs     []*config2.Channel

	Metrics      *metrics.Metrics
	Ordering     driver.Ordering
	NewChannel   NewChannelFunc
	ChannelMap   map[string]driver.Channel
	ChannelMutex sync.RWMutex
}

func NewNetwork(
	sp view2.ServiceProvider,
	name string,
	config *config2.Config,
	idProvider driver.IdentityProvider,
	localMembership driver.LocalMembership,
	sigService driver.SignerService,
	metrics *metrics.Metrics,
	newChannel NewChannelFunc,
) (*Network, error) {
	return &Network{
		SP:              sp,
		name:            name,
		config:          config,
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

func (f *Network) DefaultChannel() string {
	return f.defaultChannel
}

func (f *Network) Channels() []string {
	var chs []string
	for _, c := range f.channelConfigs {
		chs = append(chs, c.Name)
	}
	return chs
}

func (f *Network) Orderers() []*grpc.ConnectionConfig {
	return f.orderers
}

func (f *Network) PickOrderer() *grpc.ConnectionConfig {
	if len(f.orderers) == 0 {
		return nil
	}
	return f.orderers[rand.Intn(len(f.orderers))]
}

func (f *Network) Peers() []*grpc.ConnectionConfig {
	var peers []*grpc.ConnectionConfig
	for _, configs := range f.peers {
		peers = append(peers, configs...)
	}
	return peers
}

func (f *Network) PickPeer(ft driver.PeerFunctionType) *grpc.ConnectionConfig {
	source, ok := f.peers[ft]
	if !ok {
		source = f.peers[driver.PeerForAnything]
	}
	return source[rand.Intn(len(source))]
}

func (f *Network) Channel(name string) (driver.Channel, error) {
	logger.Debugf("Getting Channel [%s]", name)

	if len(name) == 0 {
		name = f.DefaultChannel()
		logger.Debugf("Resorting to default Channel [%s]", name)
	}

	chanQuiet := false
	for _, chanDef := range f.channelConfigs {
		if chanDef.Name == name {
			chanQuiet = chanDef.Quiet
			break
		}
	}

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

func (f *Network) Broadcast(context context.Context, blob interface{}) error {
	return f.Ordering.Broadcast(context, blob)
}

func (f *Network) SignerService() driver.SignerService {
	return f.sigService
}

func (f *Network) ConfigService() driver.ConfigService {
	return f.config
}

func (f *Network) Config() *config2.Config {
	return f.config
}

func (f *Network) Init() error {
	f.processorManager = rwset.NewProcessorManager(f.SP, f, nil)
	f.transactionManager = transaction.NewManager(f.SP, f)

	var err error
	f.orderers, err = f.config.Orderers()
	if err != nil {
		return errors.Wrap(err, "failed loading orderers")
	}
	f.configuredOrderers = len(f.orderers)
	logger.Debugf("Orderers [%v]", f.orderers)

	f.peers, err = f.config.Peers()
	if err != nil {
		return errors.Wrap(err, "failed loading peers")
	}
	logger.Debugf("Peers [%v]", f.peers)

	f.channelConfigs, err = f.config.Channels()
	if err != nil {
		return errors.Wrap(err, "failed loading channels")
	}
	logger.Debugf("Channels [%v]", f.channelConfigs)
	for _, channel := range f.channelConfigs {
		if channel.Default {
			f.defaultChannel = channel.Name
			break
		}
	}

	f.Ordering = ordering.NewService(f.SP, f, f.config.OrdererConnectionPoolSize(), f.Metrics)
	return nil
}

func (f *Network) setConfigOrderers(orderers []*grpc.ConnectionConfig) {
	// the first configuredOrderers are from the configuration, keep them
	// and append the new ones
	f.orderers = append(f.orderers[:f.configuredOrderers], orderers...)
	logger.Debugf("New Orderers [%d]", len(f.orderers))
}
