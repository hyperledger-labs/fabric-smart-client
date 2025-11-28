/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"
	"reflect"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger/fabric-x-committer/api/protonotify"
)

type ListenerManager interface {
	AddFinalityListener(txID driver.TxID, listener fabric.FinalityListener) error
	RemoveFinalityListener(txID driver.TxID, listener fabric.FinalityListener) error
	Listen() error
}

type ListenerManagerProvider interface {
	NewManager(ctx context.Context, network, channel string) (ListenerManager, error)
}

func NewListenerManagerProvider(fnsp *fabric.NetworkServiceProvider, configProvider config.Provider) ListenerManagerProvider {
	return &listenerManagerProvider{
		fnsp:           fnsp,
		configProvider: configProvider,
		managers:       make(map[string]ListenerManager),
	}
}

type listenerManagerProvider struct {
	fnsp           *fabric.NetworkServiceProvider
	configProvider config.Provider
	managers       map[string]ListenerManager // Cache: "network:channel" -> ListenerManager
	lock           sync.Mutex
}

func (p *listenerManagerProvider) NewManager(ctx context.Context, network, channel string) (ListenerManager, error) {
	key := network + ":" + channel

	p.lock.Lock()
	defer p.lock.Unlock()

	if lm, ok := p.managers[key]; ok {
		logger.Infof("manager is already cached")
		return lm, nil
	}

	nw, err := p.fnsp.FabricNetworkService(network)
	if err != nil {
		return nil, err
	}

	cfg, err := p.configProvider.GetConfig(nw.Name())
	if err != nil {
		return nil, err
	}

	lm, err := newNotifi(ctx, cfg)
	if err != nil {
		return nil, err
	}

	// cache the newly created instance
	p.managers[key] = lm
	logger.Infof("manager is created")

	return lm, nil
}

func newNotifi(ctx context.Context, cfg config.ConfigService) (*notificationListenerManager, error) {
	c, err := NewConfig(cfg)
	if err != nil {
		return nil, err
	}

	cc, err := GrpcClient(c)
	if err != nil {
		return nil, err
	}
	// CRITICAL FIX: Use context.Background() for the stream lifecycle.
	// This ensures the stream stays alive for the duration of the shared manager,
	// regardless of when the individual view's context (ctx) is cancelled.
	streamCtx := context.Background()

	notifyClient := protonotify.NewNotifierClient(cc)
	notifyStream, err := notifyClient.OpenNotificationStream(streamCtx)
	if err != nil {
		return nil, err
	}

	nlm := &notificationListenerManager{
		notifyStream:  notifyStream,
		requestQueue:  make(chan *protonotify.NotificationRequest, 1),
		responseQueue: make(chan *protonotify.NotificationResponse, 1),
		handlers:      make(map[string][]fabric.FinalityListener),
	}
	go func() {
		err := nlm.Listen()
		if err != nil && !errors.Is(err, context.Canceled) {
			assert.NoError(err)
		}
	}()
	return nlm, nil
}

func GetListenerManager(ctx context.Context, sp services.Provider, network, channel string) (ListenerManager, error) {
	lmp, err := sp.GetService(reflect.TypeOf((*ListenerManagerProvider)(nil)))
	if err != nil {
		return nil, errors.Wrapf(err, "could not find provider")
	}
	return lmp.(ListenerManagerProvider).NewManager(ctx, network, channel)
}
