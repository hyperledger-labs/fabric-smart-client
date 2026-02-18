/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	mock2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/finality/mock"
	"github.com/hyperledger/fabric-x-common/api/committerpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// To re-generate the mocks, run "go generate ./listenermanager_test.go"
//go:generate counterfeiter -o mock/service_provider.go --fake-name FakeServicesProvider github.com/hyperledger-labs/fabric-smart-client/platform/view/services.Provider

const (
	managerTimeout = 2 * time.Second
	managerTick    = 10 * time.Millisecond
)

// setupMockNotificationManager creates a notificationListenerManager with mocked gRPC components
// This allows us to test the REAL listen() behavior with controlled stream responses
func setupMockNotificationManager(t *testing.T, streamBehavior func(context.Context, *mock2.FakeNotifier_OpenNotificationStreamClient)) *notificationListenerManager {
	t.Helper()

	fakeStream := &mock2.FakeNotifier_OpenNotificationStreamClient{}
	fakeClient := &mock2.FakeNotifierClient{}

	// Configure the client to return our fake stream
	fakeClient.OpenNotificationStreamStub = func(c context.Context, opts ...grpc.CallOption) (committerpb.Notifier_OpenNotificationStreamClient, error) {
		fakeStream.ContextReturns(c)

		// Apply custom stream behavior if provided
		if streamBehavior != nil {
			streamBehavior(c, fakeStream)
		}

		return fakeStream, nil
	}

	nlm := &notificationListenerManager{
		notifyClient:  fakeClient,
		requestQueue:  make(chan *committerpb.NotificationRequest),
		responseQueue: make(chan *committerpb.NotificationResponse),
		handlers:      make(map[string][]fabric.FinalityListener),
	}

	return nlm
}

func TestProvider_Initialize(t *testing.T) {
	t.Run("Sets_BaseContext_On_First_Call", func(t *testing.T) {
		t.Parallel()
		provider := NewListenerManagerProvider(nil, nil)

		ctx := t.Context()
		provider.Initialize(ctx)

		require.Same(t, ctx, provider.baseCtx, "baseCtx should match the provided context")
	})

	t.Run("Initialize_Only_Once_Ignores_Subsequent_Calls", func(t *testing.T) {
		t.Parallel()
		provider := NewListenerManagerProvider(nil, nil)

		ctx1 := t.Context()
		ctx2, cancel2 := context.WithCancel(context.Background())
		defer cancel2()

		provider.Initialize(ctx1)
		provider.Initialize(ctx2) // Should be ignored due to sync.Once

		require.Same(t, ctx1, provider.baseCtx, "baseCtx should remain as first initialized value")
		require.NotSame(t, ctx2, provider.baseCtx, "Second Initialize call should be ignored")
	})

	t.Run("Thread_Safe_Concurrent_Initialize", func(t *testing.T) {
		t.Parallel()
		provider := NewListenerManagerProvider(nil, nil)

		ctx := context.Background()

		var wg sync.WaitGroup
		for range 100 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				provider.Initialize(ctx)
			}()
		}
		wg.Wait()

		require.NotNil(t, provider.baseCtx, "baseCtx should be set after concurrent Initialize calls")
	})
}

func TestProvider_NewManager(t *testing.T) {
	t.Run("Panics_If_Not_Initialized", func(t *testing.T) {
		t.Parallel()
		provider := NewListenerManagerProvider(nil, nil)

		// Don't call Initialize
		require.Panics(t, func() {
			_, _ = provider.NewManager("network1", "channel1")
		}, "Should panic when Provider is not initialized")
	})

	t.Run("Creates_New_Manager_And_Starts_Listen", func(t *testing.T) {
		t.Parallel()
		provider := NewListenerManagerProvider(nil, nil)

		listenStarted := make(chan struct{})

		// Mock with stream that blocks until context is done
		provider.newNotificationManager = func(network string, fnsp *fabric.NetworkServiceProvider, cp config.Provider) (*notificationListenerManager, error) {
			return setupMockNotificationManager(t, func(ctx context.Context, stream *mock2.FakeNotifier_OpenNotificationStreamClient) {
				stream.RecvStub = func() (*committerpb.NotificationResponse, error) {
					close(listenStarted)
					<-ctx.Done()
					return nil, ctx.Err()
				}
			}), nil
		}

		ctx := t.Context()
		provider.Initialize(ctx)

		manager, err := provider.NewManager("network1", "channel1")

		require.NoError(t, err, "NewManager should succeed")
		require.NotNil(t, manager, "Manager should not be nil")

		// Verify the REAL listen goroutine started
		select {
		case <-listenStarted:
			// Success - the real listen() is running
		case <-time.After(managerTimeout):
			t.Fatal("Real listen goroutine did not start")
		}

		// Verify manager is stored
		provider.managersMu.Lock()
		storedManager := provider.managers["network1:channel1"]
		provider.managersMu.Unlock()

		require.Same(t, manager, storedManager, "Manager should be stored in map with correct key")
	})

	t.Run("Returns_Existing_Manager_Singleton_Pattern", func(t *testing.T) {
		t.Parallel()
		provider := NewListenerManagerProvider(nil, nil)

		// Mock with stream that blocks
		provider.newNotificationManager = func(network string, fnsp *fabric.NetworkServiceProvider, cp config.Provider) (*notificationListenerManager, error) {
			return setupMockNotificationManager(t, func(ctx context.Context, stream *mock2.FakeNotifier_OpenNotificationStreamClient) {
				stream.RecvStub = func() (*committerpb.NotificationResponse, error) {
					<-ctx.Done()
					return nil, ctx.Err()
				}
			}), nil
		}

		ctx := t.Context()
		provider.Initialize(ctx)

		// Create first manager
		manager1, err1 := provider.NewManager("network1", "channel1")
		require.NoError(t, err1)

		// Wait for goroutine to start

		// Request same manager again
		manager2, err2 := provider.NewManager("network1", "channel1")
		require.NoError(t, err2)

		// Should be the exact same instance
		require.Same(t, manager1, manager2, "Should return same manager instance for same network:channel")
		require.Equal(t, 1, len(provider.managers), "Should only create manager once")
	})

	t.Run("Different_Network_Channel_Get_Different_Managers", func(t *testing.T) {
		t.Parallel()
		provider := NewListenerManagerProvider(nil, nil)

		provider.newNotificationManager = func(network string, fnsp *fabric.NetworkServiceProvider, cp config.Provider) (*notificationListenerManager, error) {
			return setupMockNotificationManager(t, func(ctx context.Context, stream *mock2.FakeNotifier_OpenNotificationStreamClient) {
				stream.RecvStub = func() (*committerpb.NotificationResponse, error) {
					<-ctx.Done()
					return nil, ctx.Err()
				}
			}), nil
		}

		ctx := t.Context()
		provider.Initialize(ctx)

		manager1, err1 := provider.NewManager("network1", "channel1")
		require.NoError(t, err1)

		manager2, err2 := provider.NewManager("network1", "channel2")
		require.NoError(t, err2)

		manager3, err3 := provider.NewManager("network2", "channel1")
		require.NoError(t, err3)

		// All should be different instances
		require.NotSame(t, manager1, manager2, "Different channels should have different managers")
		require.NotSame(t, manager1, manager3, "Different networks should have different managers")
		require.NotSame(t, manager2, manager3, "Different combinations should have different managers")

		require.Equal(t, 3, len(provider.managers), "Should create 3 different managers")
	})

	t.Run("Manager_Removed_When_Listen_Exits_With_Error", func(t *testing.T) {
		t.Parallel()
		provider := NewListenerManagerProvider(nil, nil)

		listenStarted := make(chan struct{})

		// Mock stream that returns error immediately
		provider.newNotificationManager = func(network string, fnsp *fabric.NetworkServiceProvider, cp config.Provider) (*notificationListenerManager, error) {
			return setupMockNotificationManager(t, func(ctx context.Context, stream *mock2.FakeNotifier_OpenNotificationStreamClient) {
				stream.RecvStub = func() (*committerpb.NotificationResponse, error) {
					close(listenStarted)
					return nil, errors.New("stream error")
				}
			}), nil
		}

		ctx := t.Context()
		provider.Initialize(ctx)

		_, err := provider.NewManager("network1", "channel1")
		require.NoError(t, err)

		// Wait for listen to start
		<-listenStarted

		// Wait for cleanup - the REAL listen() will exit and trigger cleanup
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			provider.managersMu.Lock()
			defer provider.managersMu.Unlock()
			_, exists := provider.managers["network1:channel1"]
			require.False(collect, exists, "Manager should be removed from map")
		}, managerTimeout, managerTick, "Manager should be removed after listen() exits with error")
	})

	t.Run("Manager_Removed_When_Context_Canceled", func(t *testing.T) {
		t.Parallel()
		provider := NewListenerManagerProvider(nil, nil)

		listenStarted := make(chan struct{})

		// Mock stream that blocks until context canceled
		provider.newNotificationManager = func(network string, fnsp *fabric.NetworkServiceProvider, cp config.Provider) (*notificationListenerManager, error) {
			return setupMockNotificationManager(t, func(ctx context.Context, stream *mock2.FakeNotifier_OpenNotificationStreamClient) {
				stream.RecvStub = func() (*committerpb.NotificationResponse, error) {
					close(listenStarted)
					<-ctx.Done()
					return nil, ctx.Err()
				}
			}), nil
		}

		ctx, cancel := context.WithCancel(context.Background())
		provider.Initialize(ctx)

		_, err := provider.NewManager("network1", "channel1")
		require.NoError(t, err)

		// Wait for listen to start
		<-listenStarted

		// Cancel context to trigger cleanup
		cancel()

		// The REAL listen() will handle context cancellation and exit gracefully
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			provider.managersMu.Lock()
			defer provider.managersMu.Unlock()
			_, exists := provider.managers["network1:channel1"]
			require.False(collect, exists, "Manager should be removed from map")
		}, managerTimeout, managerTick, "Manager should be removed after context cancellation")
	})

	t.Run("NewManager_Handles_Creation_Error", func(t *testing.T) {
		t.Parallel()
		provider := NewListenerManagerProvider(nil, nil)

		expectedErr := errors.New("failed to create manager")

		// Mock that returns error
		provider.newNotificationManager = func(network string, fnsp *fabric.NetworkServiceProvider, cp config.Provider) (*notificationListenerManager, error) {
			return nil, expectedErr
		}

		ctx := t.Context()
		provider.Initialize(ctx)

		manager, err := provider.NewManager("network1", "channel1")

		require.Error(t, err, "Should return error from newNotificationManager")
		require.Nil(t, manager, "Manager should be nil on error")
		require.Equal(t, expectedErr, err, "Should return the exact error")

		// Manager should not be stored
		provider.managersMu.Lock()
		_, exists := provider.managers["network1:channel1"]
		provider.managersMu.Unlock()

		require.False(t, exists, "Failed manager should not be stored")
	})

	t.Run("Concurrent_NewManager_Calls_Same_Key", func(t *testing.T) {
		t.Parallel()
		provider := NewListenerManagerProvider(nil, nil)

		provider.newNotificationManager = func(network string, fnsp *fabric.NetworkServiceProvider, cp config.Provider) (*notificationListenerManager, error) {
			return setupMockNotificationManager(t, func(ctx context.Context, stream *mock2.FakeNotifier_OpenNotificationStreamClient) {
				stream.RecvStub = func() (*committerpb.NotificationResponse, error) {
					<-ctx.Done()
					return nil, ctx.Err()
				}
			}), nil
		}

		ctx := t.Context()
		provider.Initialize(ctx)

		// Launch multiple concurrent requests for same manager
		const numGoroutines = 10
		managers := make([]ListenerManager, numGoroutines)
		var wg sync.WaitGroup

		for i := range numGoroutines {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				mgr, err := provider.NewManager("network1", "channel1")
				require.NoError(t, err)
				managers[idx] = mgr
			}(i)
		}
		wg.Wait()

		// All should be the same instance
		for i := 1; i < numGoroutines; i++ {
			require.Equal(t, managers[0], managers[i], "All concurrent calls should get same manager")
		}

		// Should only create manager once
		require.Equal(t, len(provider.managers), 1, "Should create manager only once even with concurrent access")
	})
}

func TestGetListenerManager(t *testing.T) {
	t.Run("Returns_Manager_From_Service_Provider", func(t *testing.T) {
		t.Parallel()

		mockSP := &mock2.FakeServicesProvider{}

		provider := NewListenerManagerProvider(nil, nil)
		provider.newNotificationManager = func(network string, fnsp *fabric.NetworkServiceProvider, cp config.Provider) (*notificationListenerManager, error) {
			return setupMockNotificationManager(t, func(ctx context.Context, stream *mock2.FakeNotifier_OpenNotificationStreamClient) {
				stream.RecvStub = func() (*committerpb.NotificationResponse, error) {
					<-ctx.Done()
					return nil, ctx.Err()
				}
			}), nil
		}

		ctx := t.Context()
		provider.Initialize(ctx)

		mockSP.GetServiceStub = func(serviceType any) (any, error) {
			return provider, nil
		}

		manager, err := GetListenerManager(mockSP, "network1", "channel1")

		require.NoError(t, err, "GetListenerManager should succeed")
		require.NotNil(t, manager, "Manager should not be nil")

		// Verify GetService was called with correct type
		require.Equal(t, 1, mockSP.GetServiceCallCount())
		serviceType := mockSP.GetServiceArgsForCall(0)
		require.Equal(t, reflect.TypeOf((*ListenerManagerProvider)(nil)), serviceType)
	})

	t.Run("Returns_Error_When_Service_Not_Found", func(t *testing.T) {
		t.Parallel()

		mockSP := &mock2.FakeServicesProvider{}
		expectedErr := errors.New("service not found")

		mockSP.GetServiceStub = func(serviceType any) (any, error) {
			return nil, expectedErr
		}

		manager, err := GetListenerManager(mockSP, "network1", "channel1")

		require.Error(t, err, "Should return error when service not found")
		require.Nil(t, manager, "Manager should be nil on error")
		require.Contains(t, err.Error(), "could not find provider")
	})

	t.Run("Returns_Error_When_NewManager_Fails", func(t *testing.T) {
		t.Parallel()

		mockSP := &mock2.FakeServicesProvider{}

		provider := NewListenerManagerProvider(nil, nil)
		expectedErr := errors.New("creation failed")

		provider.newNotificationManager = func(network string, fnsp *fabric.NetworkServiceProvider, cp config.Provider) (*notificationListenerManager, error) {
			return nil, expectedErr
		}

		ctx := t.Context()
		provider.Initialize(ctx)

		mockSP.GetServiceStub = func(serviceType any) (any, error) {
			return provider, nil
		}

		manager, err := GetListenerManager(mockSP, "network1", "channel1")

		require.Error(t, err, "Should return error from NewManager")
		require.Nil(t, manager)
		require.Equal(t, expectedErr, err)
	})
}
