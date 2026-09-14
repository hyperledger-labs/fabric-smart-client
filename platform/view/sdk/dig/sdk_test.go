/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dig2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/operations"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	viewgrpcserver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	viewmock "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/p2p"
	p2pmock "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/p2p/mock"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type failingSDK struct {
	dig2.SDK
	installErr   error
	startErr     error
	postStartErr error
}

func (f *failingSDK) Install() error {
	if f.installErr != nil {
		return f.installErr
	}
	return f.SDK.Install()
}

func (f *failingSDK) Start(ctx context.Context) error {
	if f.startErr != nil {
		return f.startErr
	}
	return f.SDK.Start(ctx)
}

func (f *failingSDK) PostStart(ctx context.Context) error {
	if f.postStartErr != nil {
		return f.postStartErr
	}
	return f.SDK.PostStart(ctx)
}

func TestWiring(t *testing.T) {
	t.Parallel()
	require.NoError(t, DryRunWiring(digutils.Identity[dig2.SDK]()))
}

func TestWiring_WithOptions(t *testing.T) {
	t.Parallel()
	require.NoError(t, DryRunWiring(
		digutils.Identity[dig2.SDK](),
		WithBool("test.bool", true),
		WithString("test.string", "foo"),
	))
}

func TestNewSDK(t *testing.T) {
	t.Parallel()

	registry := view.NewServiceProvider()
	cfg := providerFrom(t, "")
	require.NoError(t, registry.RegisterService(cfg))

	s := NewSDK(registry)
	require.NotNil(t, s)
	require.NotNil(t, s.Container())
}

func TestNewSDKFromContainer(t *testing.T) {
	t.Parallel()

	registry := view.NewServiceProvider()
	cfg := providerFrom(t, "")
	require.NoError(t, registry.RegisterService(cfg))

	c := NewContainer()
	s := NewSDKFromContainer(c, registry)
	require.NotNil(t, s)
	require.Equal(t, c, s.Container())
}

func TestNewSDKFrom_PanicOnDuplicate(t *testing.T) {
	t.Parallel()

	c := NewContainer()
	// Pre-provide services.Registry to force a duplicate provision collision in NewSDKFrom
	require.NoError(t, c.Provide(func() services.Registry { return nil }))

	registry := view.NewServiceProvider()
	cfg := providerFrom(t, "")
	require.NoError(t, registry.RegisterService(cfg))

	baseSDK := dig2.NewBaseSDK(c, cfg)
	require.Panics(t, func() {
		NewSDKFrom(baseSDK, registry)
	})
}

func TestSDK_Install_ProvideError(t *testing.T) {
	t.Parallel()

	c := NewContainer()
	// Pre-provide *operations.Options so that p.Container().Provide(NewOperationsOptions) fails with duplicate provide
	require.NoError(t, c.Provide(func() *operations.Options { return nil }))

	registry := view.NewServiceProvider()
	cfg := providerFrom(t, "")
	require.NoError(t, registry.RegisterService(cfg))

	baseSDK := dig2.NewBaseSDK(c, cfg)
	s := NewSDKFrom(baseSDK, registry)

	err := s.Install()
	require.Error(t, err)
}

func TestSDK_Install_ParentError(t *testing.T) {
	t.Parallel()

	c := NewContainer()
	registry := view.NewServiceProvider()
	cfg := providerFrom(t, "")
	require.NoError(t, registry.RegisterService(cfg))

	parentErr := errors.New("parent-install-failed")
	baseSDK := dig2.NewBaseSDK(c, cfg)
	s := NewSDKFrom(&failingSDK{SDK: baseSDK, installErr: parentErr}, registry)

	err := s.Install()
	require.ErrorIs(t, err, parentErr)
}

func TestSDK_Install_RegistrationError(t *testing.T) {
	t.Parallel()

	c := NewContainer()
	registry := view.NewServiceProvider()
	cfg := providerFrom(t, "")
	require.NoError(t, registry.RegisterService(cfg))

	baseSDK := dig2.NewBaseSDK(c, cfg)
	s := NewSDKFrom(baseSDK, registry)

	err := s.Install()
	require.ErrorContains(t, err, "failed registering type")
}

func TestSDK_Start_RemovedKeys(t *testing.T) {
	t.Parallel()

	c := NewContainer()
	registry := view.NewServiceProvider()
	cfg := providerFrom(t, `
fsc:
  metrics:
    prometheus:
      tls: true
`)
	require.NoError(t, registry.RegisterService(cfg))

	baseSDK := dig2.NewBaseSDK(c, cfg)
	s := NewSDKFrom(baseSDK, registry)

	err := s.Start(t.Context())
	require.ErrorContains(t, err, "configuration key [fsc.metrics.prometheus.tls] has been removed")
}

func TestSDK_Start_ParentError(t *testing.T) {
	t.Parallel()

	c := NewContainer()
	registry := view.NewServiceProvider()
	cfg := providerFrom(t, "")
	require.NoError(t, registry.RegisterService(cfg))

	parentErr := errors.New("parent-start-failed")
	baseSDK := dig2.NewBaseSDK(c, cfg)
	s := NewSDKFrom(&failingSDK{SDK: baseSDK, startErr: parentErr}, registry)

	err := s.Start(t.Context())
	require.ErrorIs(t, err, parentErr)
}

func TestSDK_Start_MissingDependencies(t *testing.T) {
	t.Parallel()

	c := NewContainer()
	registry := view.NewServiceProvider()
	cfg := providerFrom(t, "")
	require.NoError(t, registry.RegisterService(cfg))

	baseSDK := dig2.NewBaseSDK(c, cfg)
	s := NewSDKFrom(baseSDK, registry)

	// Invoke in Start requires GRPCServer, ViewManager, etc., which have not been installed
	err := s.Start(t.Context())
	require.Error(t, err)
}

type fakeP2PViewManager struct{}

func (*fakeP2PViewManager) ExistResponderForCaller(string) (view2.View, view2.Identity, error) {
	return nil, nil, nil
}

func (*fakeP2PViewManager) NewResponderContext(context.Context, string, view2.Session, view2.Identity, view2.Identity) (view2.Context, bool, error) {
	return nil, false, nil
}

func (*fakeP2PViewManager) DeleteContext(string) {}

type fakeViewService struct {
	protos.UnimplementedViewServiceServer
	registered bool
}

func (f *fakeViewService) RegisterProcessor(reflect.Type, viewgrpcserver.Processor) {
	f.registered = true
}

func (*fakeViewService) RegisterStreamer(reflect.Type, viewgrpcserver.Streamer) {}

type fakeHostProvider struct{}

func (*fakeHostProvider) GetNewHost() (host.P2PHost, error) {
	return nil, errors.New("cannot get host")
}

func wireSDKDeps(t *testing.T, c *baseContainer, tp TracerProviders, commMock p2p.CommLayer) *fakeViewService {
	t.Helper()

	p2pSvc := p2p.NewService(&fakeP2PViewManager{}, &fakeIdentityProvider{}, commMock, nil, nil)
	commSvc := &comm.Service{HostProvider: &fakeHostProvider{}}
	viewSvc := &fakeViewService{}

	require.NoError(t, errors.Join(
		c.Provide(func() *grpc.GRPCServer { return nil }),
		c.Provide(func() viewgrpcserver.ViewManager { return &fakeViewManager{} }),
		c.Provide(func() *p2p.Service { return p2pSvc }),
		c.Provide(func() viewgrpcserver.Service { return viewSvc }),
		c.Provide(func() *comm.Service { return commSvc }),
		c.Provide(func() Server { return newFakeServer() }),
		c.Provide(func() OperationsServer { return OperationsServer{} }),
		c.Provide(func() *operations.System { return nil }),
		c.Provide(func() *kvs.KVS { return nil }),
		c.Provide(func() tracing.Provider { return tp.Default }),
		c.Provide(func() viewgrpcserver.IdentityProvider { return &fakeIdentityProvider{} }),
	))

	return viewSvc
}

func TestSDK_Start_P2PFailure(t *testing.T) {
	t.Parallel()

	c := NewContainer()
	registry := view.NewServiceProvider()
	cfg := providerFrom(t, "")
	require.NoError(t, registry.RegisterService(cfg))

	baseSDK := dig2.NewBaseSDK(c, cfg)
	s := NewSDKFrom(baseSDK, registry)

	commLayerErr := errors.New("p2p-master-session-failure")
	commMock := &p2pmock.CommLayer{}
	commMock.MasterSessionReturns(nil, commLayerErr)
	tp, err := newTracerProvider(&disabled.Provider{}, cfg)
	require.NoError(t, err)

	wireSDKDeps(t, c, tp, commMock)

	err = s.Start(t.Context())
	require.ErrorContains(t, err, "failed getting master session")
}

func TestSDK_Start_And_PostStart_Success(t *testing.T) {
	t.Parallel()

	c := NewContainer()
	registry := view.NewServiceProvider()
	cfg := providerFrom(t, `
fsc:
  grpc:
    enabled: false
`)
	require.NoError(t, registry.RegisterService(cfg))

	baseSDK := dig2.NewBaseSDK(c, cfg)
	s := NewSDKFrom(baseSDK, registry)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	sess := &viewmock.Session{}
	sess.ReceiveReturns(make(chan *view2.Message))
	commMock := &p2pmock.CommLayer{}
	commMock.MasterSessionReturns(sess, nil)
	tp, err := newTracerProvider(&disabled.Provider{}, cfg)
	require.NoError(t, err)

	viewSvc := wireSDKDeps(t, c, tp, commMock)

	require.NoError(t, s.Start(ctx))
	assert.True(t, viewSvc.registered)

	// PostStart calls Visualize() and delegates to parent
	require.NoError(t, s.PostStart(ctx))
}

func TestSDK_PostStart_ParentError(t *testing.T) {
	t.Parallel()

	c := NewContainer()
	registry := view.NewServiceProvider()
	cfg := providerFrom(t, "")
	require.NoError(t, registry.RegisterService(cfg))

	parentErr := errors.New("parent-poststart-failed")
	baseSDK := dig2.NewBaseSDK(c, cfg)
	s := NewSDKFrom(&failingSDK{SDK: baseSDK, postStartErr: parentErr}, registry)

	err := s.PostStart(t.Context())
	require.ErrorIs(t, err, parentErr)
}
