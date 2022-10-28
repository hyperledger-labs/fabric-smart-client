/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"math/rand"
	"sync"

	config2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
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

type Network struct {
	sp view2.ServiceProvider

	config *config2.Config

	localMembership    driver.LocalMembership
	idProvider         driver.IdentityProvider
	processorManager   driver.ProcessorManager
	transactionManager driver.TransactionManager
	sigService         driver.SignerService

	orderers           []*grpc.ConnectionConfig
	configuredOrderers int
	peers              []*grpc.ConnectionConfig
	defaultChannel     string
	channelDefs        []*config2.Channel

	ordering driver.Ordering
	channels map[string]driver.Channel
	mutex    sync.RWMutex
	name     string
}

func NewNetwork(
	sp view2.ServiceProvider,
	name string,
	config *config2.Config,
	idProvider driver.IdentityProvider,
	localMembership driver.LocalMembership,
	sigService driver.SignerService,
) (*Network, error) {
	// Load configuration
	fsp := &Network{
		sp:              sp,
		name:            name,
		config:          config,
		channels:        map[string]driver.Channel{},
		localMembership: localMembership,
		idProvider:      idProvider,
		sigService:      sigService,
	}
	err := fsp.init()
	if err != nil {
		return nil, err
	}
	return fsp, nil
}

func (f *Network) Name() string {
	return f.name
}

func (f *Network) DefaultChannel() string {
	return f.defaultChannel
}

func (f *Network) Channels() []string {
	var chs []string
	for _, c := range f.channelDefs {
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
	return f.peers
}

func (f *Network) PickPeer() *grpc.ConnectionConfig {
	return f.peers[rand.Intn(len(f.peers))]
}

func (f *Network) Channel(name string) (driver.Channel, error) {
	logger.Debugf("Getting channel [%s]", name)

	if len(name) == 0 {
		name = f.DefaultChannel()
		logger.Debugf("Resorting to default channel [%s]", name)
	}

	chanQuiet := false
	for _, chanDef := range f.channelDefs {
		if chanDef.Name == name {
			chanQuiet = chanDef.Quiet
			break
		}
	}

	// first check the cache
	f.mutex.RLock()
	ch, ok := f.channels[name]
	f.mutex.RUnlock()
	if ok {
		logger.Debugf("Returning channel for [%s]", name)
		return ch, nil
	}

	// create channel and store in cache
	f.mutex.Lock()
	defer f.mutex.Unlock()

	ch, ok = f.channels[name]
	if !ok {
		logger.Debugf("Channel [%s] not found, allocate resources", name)
		var err error
		ch, err = newChannel(f, name, chanQuiet)
		if err != nil {
			return nil, err
		}
		f.channels[name] = ch
		logger.Debugf("Channel [%s] not found, created", name)
	}

	logger.Debugf("Returning channel for [%s]", name)
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

func (f *Network) Broadcast(blob interface{}) error {
	return f.ordering.Broadcast(blob)
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

func (f *Network) init() error {
	f.processorManager = rwset.NewProcessorManager(f.sp, f, nil)
	f.transactionManager = transaction.NewManager(f.sp, f)

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

	f.channelDefs, err = f.config.Channels()
	if err != nil {
		return errors.Wrap(err, "failed loading channels")
	}
	logger.Debugf("Channels [%v]", f.channelDefs)
	for _, channel := range f.channelDefs {
		if channel.Default {
			f.defaultChannel = channel.Name
			break
		}
	}

	f.ordering = ordering.NewService(f.sp, f, f.transactionManager)
	return nil
}

func (f *Network) setConfigOrderers(orderers []*grpc.ConnectionConfig) {
	// the first configuredOrderers are from the configuration, keep them
	// and append the new ones
	f.orderers = append(f.orderers[:f.configuredOrderers], orderers...)
	logger.Debugf("New Orderers [%d]", len(f.orderers))
}
