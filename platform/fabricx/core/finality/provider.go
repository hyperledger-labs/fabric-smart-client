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
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger/fabric-x-common/api/committerpb"
)

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

// NewListenerManagerProvider creates a new instance of the Provider, which implements ListenerManagerProvider.
// This provider manages the lifecycle of ListenerManager instances, ensuring one per
// network/channel combination.
func NewListenerManagerProvider(fnsp *fabric.NetworkServiceProvider, configProvider config.Provider) *Provider {
	return &Provider{
		fnsp:                   fnsp,
		configProvider:         configProvider,
		managers:               make(map[string]ListenerManager),
		newNotificationManager: newNotifi,
		// Note: baseCtx will be initialized in the Initialize method.
	}
}

// Provider implements ListenerManagerProvider and manages ListenerManager instances.
// IMPORTANT: Initialize method MUST be called once during service setup before calling NewManager method.
type Provider struct {
	newNotificationManager func(network string, fnsp *fabric.NetworkServiceProvider, cp config.Provider) (*notificationListenerManager, error)
	fnsp                   *fabric.NetworkServiceProvider
	configProvider         config.Provider
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
	lm, err := p.newNotificationManager(network, p.fnsp, p.configProvider)
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

// newNotifi creates and initializes a notificationListenerManager, establishing a gRPC connection
// and opening a notification stream to the external committer service.
func newNotifi(network string, fnsp *fabric.NetworkServiceProvider, configProvider config.Provider) (*notificationListenerManager, error) { // 1. Resolve the network service using the interface
	nw, err := fnsp.FabricNetworkService(network)
	if err != nil {
		return nil, err
	}

	// Load the specific configuration for this network
	cfg, err := configProvider.GetConfig(nw.Name())
	if err != nil {
		return nil, err
	}

	c, err := NewConfig(cfg)
	if err != nil {
		return nil, err
	}

	cc, err := GrpcClient(c)
	if err != nil {
		return nil, err
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
