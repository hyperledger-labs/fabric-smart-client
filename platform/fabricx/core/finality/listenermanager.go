/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
)

type ListenerManager interface {
	AddFinalityListener(txID driver.TxID, listener fabric.FinalityListener) error

	RemoveFinalityListener(txID driver.TxID, listener fabric.FinalityListener) error
}

type ListenerManagerProvider interface {
	NewManager(network, channel string) (ListenerManager, error)
}

func NewListenerManagerProvider(fnsp *fabric.NetworkServiceProvider, configProvider config.Provider) ListenerManagerProvider {
	return &listenerManagerProvider{
		fnsp:           fnsp,
		configProvider: configProvider,
	}
}

type listenerManagerProvider struct {
	fnsp           *fabric.NetworkServiceProvider
	configProvider config.Provider
}

func (p *listenerManagerProvider) NewManager(network, channel string) (ListenerManager, error) {
	nw, err := p.fnsp.FabricNetworkService(network)
	if err != nil {
		return nil, err
	}

	ch, err := nw.Channel(channel)
	if err != nil {
		return nil, err
	}

	cfg, err := p.configProvider.GetConfig(nw.Name())
	if err != nil {
		return nil, err
	}

	switch cfg.GetString("network.finality.type") {
	case "delivery":
		return finality.NewDeliveryFLM(logging.MustGetLogger("delivery-flm"), events.DeliveryListenerManagerConfig{
			MapperParallelism:       cfg.GetInt("network.finality.delivery.mapperParallelism"),
			BlockProcessParallelism: cfg.GetInt("network.finality.delivery.blockProcessParallelism"),
			ListenerTimeout:         cfg.GetDuration("network.finality.delivery.listenerTimeout"),
			LRUSize:                 cfg.GetInt("network.finality.delivery.lruSize"),
			LRUBuffer:               cfg.GetInt("network.finality.delivery.lruBuffer"),
		}, nw.Name(), ch)
	case "committer":
	case "":
		return &committerListenerManager{committer: ch.Committer()}, nil
	}
	panic("unknown finality type")
}

type committerListenerManager struct {
	committer *fabric.Committer
}

func (m *committerListenerManager) AddFinalityListener(txID driver.TxID, listener fabric.FinalityListener) error {
	return m.committer.AddFinalityListener(txID, listener)
}

func (m *committerListenerManager) RemoveFinalityListener(txID string, listener fabric.FinalityListener) error {
	return m.committer.RemoveFinalityListener(txID, listener)
}

func GetListenerManager(sp services.Provider, network, channel string) (ListenerManager, error) {
	lmp, err := sp.GetService(reflect.TypeOf((*ListenerManagerProvider)(nil)))
	if err != nil {
		return nil, errors.Wrapf(err, "could not find provider")
	}
	return lmp.(ListenerManagerProvider).NewManager(network, channel)
}
