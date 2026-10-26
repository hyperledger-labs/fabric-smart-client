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
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/deferred"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate"
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
// network/channel combination. Each manager reports the state of its notification stream to
// connState under the fabricx_notification component; a nil connState reports nothing.
func NewListenerManagerProvider(grpcClientProvider GRPCClientProvider, configProvider ServiceConfigProvider, connState *connstate.Metrics) *Provider {
	return &Provider{
		grpcClientProvider:     grpcClientProvider,
		configProvider:         configProvider,
		connState:              connState,
		managers:               make(map[string]ListenerManager),
		newNotificationManager: newNotificationManagerWithGRPC,
		baseCtx:                deferred.NewHolder[context.Context]("finality listener manager provider base context"),
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
	connState              *connstate.Metrics
	managers               map[string]ListenerManager // map: "network:channel" -> ListenerManager instance
	managersMu             sync.Mutex

	// baseCtx holds the root context for all ListenerManager goroutines,
	// installed by Initialize. A deferred.Holder rather than a plain field
	// guarded by sync.Once: Once orders its write only against goroutines that
	// call Do, and NewManager never does, so it left the read racing the write.
	baseCtx *deferred.Holder[context.Context]
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
// A new manager is served in the background until the base context ends, reconnecting
// its notification stream whenever it fails; see notificationListenerManager.run.
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
	lm.conn = p.connState.Tracker(network, channel, connstate.ComponentFabricxNotification, lm.endpoint)

	// 4. Register the newly created instance
	p.managers[key] = lm

	// 5. Serve in the background. run reconnects the notification stream itself and
	// returns only once baseCtx ends, so the manager stays registered, and usable by
	// the channel that holds it, across stream failures.
	go func() {
		logger.Debugf("Starting notification listener manager for %s", key)
		if err := lm.run(baseCtx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Errorf("Notification listener manager stopped unexpectedly for %s: %s", key, err)
		}

		p.managersMu.Lock()
		delete(p.managers, key)
		p.managersMu.Unlock()
		logger.Debugf("manager removed for %s", key)
	}()

	logger.Debugf("manager is created and listening for %s", key)

	return lm, nil
}

// newNotificationManagerWithGRPC creates and initializes a notificationListenerManager using the GRPCClientProvider.
//
// cfg is expected to already be fully resolved (see config.NewNotificationServiceConfig /
// config.DefaultConfig). The pool sizes are clamped anyway: they reach make() and the
// worker loop directly, where zero leaves nothing to drain the queue and negative panics.
func newNotificationManagerWithGRPC(network string, grpcClientProvider GRPCClientProvider, cfg config.Config) (*notificationListenerManager, error) {
	newNotifyClient := func() (committerpb.SidecarServiceClient, error) {
		cc, err := grpcClientProvider.NotificationServiceClient(network)
		if err != nil {
			return nil, errors.Wrapf(err, "get grpc client for notification service [network=%s]", network)
		}
		// Create the gRPC client stub for the committer sidecar service
		return committerpb.NewSidecarServiceClient(cc), nil
	}
	// Fail now on a broken configuration: run retries a failing newNotifyClient forever,
	// where the error would only ever surface as reconnect warnings. The endpoint picked
	// here labels the connection-state series.
	cc, err := grpcClientProvider.NotificationServiceClient(network)
	if err != nil {
		return nil, errors.Wrapf(err, "get grpc client for notification service [network=%s]", network)
	}

	nlm := &notificationListenerManager{
		newNotifyClient:  newNotifyClient,
		endpoint:         cc.Target(),
		requestQueue:     make(chan *committerpb.NotificationRequest),  // Queue for outgoing requests to the committer
		responseQueue:    make(chan *committerpb.NotificationResponse), // Queue for incoming responses/notifications
		handlers:         make(map[driver.TxID]*handlerEntry),          // Map: txID -> listeners + local expiry deadline
		handlerWorkers:   max(cfg.HandlerWorkers, 1),
		handlerQueueSize: max(cfg.HandlerQueueSize, 1),
		handlerTimeout:   cfg.HandlerTimeout,
		requestTimeout:   cfg.RequestTimeout,
		listenerTTL:      cfg.ListenerTTL,
		sweepInterval:    cfg.SweepInterval,
	}

	return nlm, nil
}

// GetListenerManager fetches the ListenerManager for the specified network and channel
// from the view service provider. It relies on the service provider to locate
// the registered ListenerManagerProvider and then delegates the creation/retrieval.
//
// This is the bare notification-stream manager: it can only report on a transaction the
// committer has not reported on yet, and answers Unknown for one it has (see
// [ResolvingListenerManager]). Applications should register through the channel's
// committer instead -- ch.Committer().AddFinalityListener -- which is the same manager
// wrapped so that an already-final transaction is resolved from the query service. Use
// this function when you specifically want the unwrapped manager.
func GetListenerManager(sp services.Provider, network, channel string) (ListenerManager, error) {
	lmp, err := sp.GetService(reflect.TypeFor[*ListenerManagerProvider]())
	if err != nil {
		return nil, errors.Wrap(err, "could not find provider")
	}
	provider, ok := lmp.(ListenerManagerProvider)
	if !ok {
		return nil, errors.Errorf("unexpected provider type [%T]", lmp)
	}
	return provider.NewManager(network, channel)
}
