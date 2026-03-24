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
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger/fabric-x-common/api/committerpb"
	"google.golang.org/grpc"
)

// GRPCClientProvider provides gRPC client connections for a given network.
type GRPCClientProvider interface {
	// NotificationServiceClient returns a gRPC client connection for the specified network.
	NotificationServiceClient(network string) (*grpc.ClientConn, error)
}

// ListenerManager defines the interface for managing finality listeners for transactions.
// It allows for dynamic registration and de-registration of callbacks (listeners)
// that are triggered when a specific transaction is finalized.
type ListenerManager interface {
	AddFinalityListener(txID driver.TxID, listener fabric.FinalityListener) error
	RemoveFinalityListener(txID driver.TxID, listener fabric.FinalityListener) error
}

// ListenerManagerProvider defines the interface for creating new ListenerManager instances
// for specific network and channel combinations.
type ListenerManagerProvider interface {
	NewManager(network, channel string) (ListenerManager, error)
}

// ServiceConfigProvider provides gRPC configuration for a given network.
//
//go:generate counterfeiter -o mock/service_config_provider.go --fake-name ServiceConfigProvider . ServiceConfigProvider
type ServiceConfigProvider interface {
	// NotificationServiceConfig returns the configuration for the notification service for the specified network.
	NotificationServiceConfig(network string) (*config.Config, error)
}

// NewListenerManagerProvider creates a new instance of the Provider, which implements ListenerManagerProvider.
// This provider manages the lifecycle of ListenerManager instances, ensuring one per
// network/channel combination.
func NewListenerManagerProvider(grpcClientProvider GRPCClientProvider, fnsp *fabric.NetworkServiceProvider, configProvider ServiceConfigProvider) *Provider {
	return &Provider{
		grpcClientProvider:     grpcClientProvider,
		fnsp:                   fnsp,
		configProvider:         configProvider,
		managers:               make(map[string]ListenerManager),
		newNotificationManager: newNotifiWithGRPC,
		// Note: baseCtx will be initialized in the Initialize method.
	}
}

// Provider implements ListenerManagerProvider and manages ListenerManager instances.
// IMPORTANT: Initialize method MUST be called once during service setup before calling NewManager method.
type Provider struct {
	newNotificationManager func(network string, fnsp *fabric.NetworkServiceProvider, gcp GRPCClientProvider) (*notificationListenerManager, error)
	fnsp                   *fabric.NetworkServiceProvider
	configProvider         ServiceConfigProvider
	grpcClientProvider     GRPCClientProvider
	managers               map[string]ListenerManager // map: "network:channel" -> ListenerManager instance
	managersMu             sync.Mutex
	baseCtx                context.Context // The root context for all ListenerManager goroutines. MUST be set via Initialize().
	initOnce               sync.Once       // Ensures the provider is initialized only once
}

// Initialize sets the base context for the provider. This context is used as the parent
// for the listening goroutines of each ListenerManager.
//
// IMPORTANT: This method MUST be called once during service setup before calling NewManager.
func (p *Provider) Initialize(ctx context.Context) {
	p.initOnce.Do(func() {
		p.baseCtx = ctx
		logger.Debug("Provider initialized with base context")
	})
}

// NewManager retrieves or creates a ListenerManager for the given network and channel.
// It ensures that only one ListenerManager exists for a specific network and channel combination
// (singleton per network/channel).
// If a new manager is created, it starts the manager's blocking listening process
// in a separate goroutine.
func (p *Provider) NewManager(network, channel string) (ListenerManager, error) {
	if p.baseCtx == nil {
		panic("programming error: Provider is not initialized. The Initialize() method must be called before NewManager.")
	}

	key := network + ":" + channel

	p.managersMu.Lock()
	defer p.managersMu.Unlock()

	// 1. Check if manager already exists
	if lm, ok := p.managers[key]; ok {
		logger.Debugf("manager is already created for %s", key)
		return lm, nil
	}

	// 2. Create the concrete ListenerManager
	lm, err := p.newNotificationManager(network, p.fnsp, p.grpcClientProvider)
	if err != nil {
		return nil, err
	}

	// 3. Register the newly created instance
	p.managers[key] = lm

	// 4. Start listening in background
	// lm.listen() is a blocking method that establishes and maintains a stream connection
	// to receive finality notifications.
	go func() {
		logger.Debugf("Starting notification listener stream for %s", key)
		if err := lm.listen(p.baseCtx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Errorf("Notification listener stream terminated unexpectedly for %s: %s", key, err)
		}

		// Clean up: Remove the manager from the map when listen exits
		p.managersMu.Lock()
		delete(p.managers, key)
		p.managersMu.Unlock()
		logger.Debugf("manager removed for %s", key)
	}()

	logger.Debugf("manager is created and listening for %s", key)

	return lm, nil
}

// newNotifiWithGRPC creates and initializes a notificationListenerManager using the GRPCClientProvider.
func newNotifiWithGRPC(network string, fnsp *fabric.NetworkServiceProvider, grpcClientProvider GRPCClientProvider) (*notificationListenerManager, error) {
	cc, err := grpcClientProvider.NotificationServiceClient(network)
	if err != nil {
		return nil, errors.Wrapf(err, "get grpc client for notification service [network=%s]", network)
	}

	// Create the gRPC client stub for the Notifier service
	notifyClient := committerpb.NewNotifierClient(cc)

	nlm := &notificationListenerManager{
		notifyClient:   notifyClient,
		requestQueue:   make(chan *committerpb.NotificationRequest),  // Queue for outgoing requests to the committer
		responseQueue:  make(chan *committerpb.NotificationResponse), // Queue for incoming responses/notifications
		handlers:       make(map[string][]fabric.FinalityListener),   // Map: txID -> list of listeners
		handlerTimeout: DefaultHandlerTimeout,
	}

	return nlm, nil
}

// GetListenerManager fetches the ListenerManager for the specified network and channel
// from the view service provider. It relies on the service provider to locate
// the registered ListenerManagerProvider and then delegates the creation/retrieval.
func GetListenerManager(sp services.Provider, network, channel string) (ListenerManager, error) {
	lmp, err := sp.GetService(reflect.TypeOf((*ListenerManagerProvider)(nil)))
	if err != nil {
		return nil, errors.Wrapf(err, "could not find provider")
	}
	return lmp.(ListenerManagerProvider).NewManager(network, channel)
}
