/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"
	"reflect"
	"sync"

	"github.com/hyperledger/fabric-x-common/api/committerpb"
	"google.golang.org/grpc"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/configstate"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
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
type ServiceConfigProvider interface {
	// NotificationServiceConfig returns the configuration for the notification service for the specified network.
	NotificationServiceConfig(network string) (*config.Config, error)
}

// NewListenerManagerProvider creates a new instance of the Provider, which implements ListenerManagerProvider.
// This provider manages the lifecycle of ListenerManager instances, ensuring one per
// network/channel combination.
func NewListenerManagerProvider(grpcClientProvider GRPCClientProvider, configProvider ServiceConfigProvider) *Provider {
	return &Provider{
		grpcClientProvider:     grpcClientProvider,
		configProvider:         configProvider,
		managers:               make(map[string]ListenerManager),
		newNotificationManager: newNotifiWithGRPC,
		baseCtx:                configstate.NewHolder[context.Context]("finality listener manager provider base context"),
	}
}

// Provider implements ListenerManagerProvider and manages ListenerManager instances.
//
// The root context for the listening goroutines is not available when the
// Provider is built; the SDK supplies it during startup, via Initialize. Until
// then NewManager reports fabric driver.ErrNotInitialized — the one in
// platform/fabric/driver, not the platform/common/driver this file imports as
// driver.
type Provider struct {
	newNotificationManager func(network string, gcp GRPCClientProvider, cfg config.Config) (*notificationListenerManager, error)
	configProvider         ServiceConfigProvider
	grpcClientProvider     GRPCClientProvider
	managers               map[string]ListenerManager // map: "network:channel" -> ListenerManager instance
	managersMu             sync.Mutex

	// baseCtx holds the root context for all ListenerManager goroutines,
	// installed by Initialize. A configstate.Holder rather than a plain field
	// guarded by sync.Once: Once orders its write only against goroutines that
	// call Do, and NewManager never does, so it left the read racing the write.
	baseCtx *configstate.Holder[context.Context]
}

// resolveConfig looks up the notification service config for network, falling
// back to config.DefaultConfig() when the Provider has no configProvider (as in
// unit tests that never reach a real network). Deliberately not called while
// holding managersMu: resolving configuration can be relatively expensive
// (a network round-trip in some ServiceConfigProvider implementations), and it
// must not be done while blocking every other network/channel's NewManager call.
func (p *Provider) resolveConfig(network string) (config.Config, error) {
	if p.configProvider == nil {
		return config.DefaultConfig(), nil
	}
	cfg, err := p.configProvider.NotificationServiceConfig(network)
	if err != nil {
		return config.Config{}, errors.Wrapf(err, "get notification service config [network=%s]", network)
	}
	return *cfg, nil
}

// Initialize sets the base context for the provider. This context is used as the parent
// for the listening goroutines of each ListenerManager.
//
// A nil context is ignored: installing one would leave the provider looking
// initialized while every listener goroutine ran on nil, which surfaces as a
// panic inside gRPC with nothing pointing back here. The provider stays
// uninitialized instead, so NewManager keeps reporting it.
//
// IMPORTANT: This method MUST be called once during service setup before calling NewManager.
func (p *Provider) Initialize(ctx context.Context) {
	if ctx == nil {
		logger.Error("Finality Provider.Initialize called with a nil context, ignoring it")
		return
	}

	// The error is discarded because the update function cannot fail; keeping
	// the first context is expressed by returning it unchanged.
	_ = p.baseCtx.Update(func(current context.Context, loaded bool) (context.Context, error) {
		if loaded {
			return current, nil
		}
		logger.Debug("Provider initialized with base context")
		return ctx, nil
	})
}

// NewManager retrieves or creates a ListenerManager for the given network and channel.
// It ensures that only one ListenerManager exists for a specific network and channel combination
// (singleton per network/channel).
// If a new manager is created, it starts the manager's blocking listening process
// in a separate goroutine.
func (p *Provider) NewManager(network, channel string) (ListenerManager, error) {
	// Resolved once here and passed to the listening goroutine below, so that
	// the manager registered in this call and the context its goroutine runs on
	// come from the same read.
	baseCtx, err := p.baseCtx.Get()
	if err != nil {
		return nil, err
	}

	key := network + ":" + channel

	// 1. Resolve the notification service config for this network. Deliberately
	// done before acquiring managersMu: resolving configuration can be relatively
	// expensive (a network round-trip in some ServiceConfigProvider
	// implementations), and it must not be done while blocking every other
	// network/channel's NewManager call.
	cfg, err := p.resolveConfig(network)
	if err != nil {
		return nil, err
	}

	p.managersMu.Lock()
	defer p.managersMu.Unlock()

	// 2. Check if manager already exists
	if lm, ok := p.managers[key]; ok {
		logger.Debugf("manager is already created for %s", key)
		return lm, nil
	}

	// 3. Create the concrete ListenerManager
	lm, err := p.newNotificationManager(network, p.grpcClientProvider, cfg)
	if err != nil {
		return nil, err
	}

	// 4. Register the newly created instance
	p.managers[key] = lm

	// 5. Start listening in background
	// lm.listen() is a blocking method that establishes and maintains a stream connection
	// to receive finality notifications.
	go func() {
		logger.Debugf("Starting notification listener stream for %s", key)
		if err := lm.listen(baseCtx); err != nil && !errors.Is(err, context.Canceled) {
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
// cfg is expected to already be fully resolved (see config.NewNotificationServiceConfig /
// config.DefaultConfig) -- every field is used as-is, with no further nil or zero-value handling.
func newNotifiWithGRPC(network string, grpcClientProvider GRPCClientProvider, cfg config.Config) (*notificationListenerManager, error) {
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
		handlers:       make(map[driver.TxID]*handlerEntry),          // Map: txID -> listeners + local expiry deadline
		handlerTimeout: cfg.HandlerTimeout,
		requestTimeout: cfg.RequestTimeout,
		listenerTTL:    cfg.ListenerTTL,
		sweepInterval:  cfg.SweepInterval,
	}

	return nlm, nil
}

// GetListenerManager fetches the ListenerManager for the specified network and channel
// from the view service provider. It relies on the service provider to locate
// the registered ListenerManagerProvider and then delegates the creation/retrieval.
func GetListenerManager(sp services.Provider, network, channel string) (ListenerManager, error) {
	lmp, err := sp.GetService(reflect.TypeFor[*ListenerManagerProvider]())
	if err != nil {
		return nil, errors.Wrapf(err, "could not find provider")
	}
	return lmp.(ListenerManagerProvider).NewManager(network, channel)
}
