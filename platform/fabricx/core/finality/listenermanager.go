/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger/fabric-x-committer/api/protonotify"
)

type ListenerManager interface {
	AddFinalityListener(txID driver.TxID, listener fabric.FinalityListener) error
	RemoveFinalityListener(txID driver.TxID, listener fabric.FinalityListener) error
	Listen(ctx context.Context) error
}

type ListenerManagerProvider interface {
	NewManager(ctx context.Context, network, channel string) (ListenerManager, error)
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

func (p *listenerManagerProvider) NewManager(ctx context.Context, network, channel string) (ListenerManager, error) {
	nw, err := p.fnsp.FabricNetworkService(network)
	if err != nil {
		return nil, err
	}

	cfg, err := p.configProvider.GetConfig(nw.Name())
	if err != nil {
		return nil, err
	}

	return newNotifi(ctx, cfg)
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

	notifyClient := protonotify.NewNotifierClient(cc)
	notifyStream, err := notifyClient.OpenNotificationStream(ctx)
	if err != nil {
		return nil, err
	}

	nlm := &notificationListenerManager{
		notifyStream:  notifyStream,
		requestQueue:  make(chan *protonotify.NotificationRequest, 1),
		responseQueue: make(chan *protonotify.NotificationResponse, 1),
		handlers:      make(map[string][]fabric.FinalityListener),
	}

	return nlm, nil
}

func GetListenerManager(ctx context.Context, sp services.Provider, network, channel string) (ListenerManager, error) {
	lmp, err := sp.GetService(reflect.TypeOf((*ListenerManagerProvider)(nil)))
	if err != nil {
		return nil, errors.Wrapf(err, "could not find provider")
	}
	return lmp.(ListenerManagerProvider).NewManager(ctx, network, channel)
}
