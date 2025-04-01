/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"context"
	"encoding/json"
	"log"
	"runtime/debug"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/api"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/registry"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

const (
	fidLabel tracing.LabelName = "fid"
)

var logger = logging.MustGetLogger("fsc")

type ExecuteCallbackFunc = func() error

type ViewRegistry interface {
	RegisterFactory(id string, factory view2.Factory) error
	RegisterResponder(responder view2.View, initiatedBy interface{}) error
	RegisterResponderWithIdentity(responder view2.View, id view.Identity, initiatedBy interface{}) error
}

type ViewManager interface {
	NewView(id string, in []byte) (view.View, error)
	InitiateView(view view.View, ctx context.Context) (interface{}, error)
	InitiateContext(view view.View) (view.Context, error)
	InitiateContextWithIdentity(view view.View, id view.Identity) (view.Context, error)
	InitiateContextFrom(ctx context.Context, view view.View, id view.Identity, contextID string) (view.Context, error)
	Context(contextID string) (view.Context, error)
}

type ServiceRegisterer interface {
	RegisterService(service interface{}) error
}

type serviceRegistry interface {
	GetService(v interface{}) (interface{}, error)

	RegisterService(service interface{}) error
}

type Registry interface {
	serviceRegistry
	ConfigService() driver.ConfigService
	RegisterViewManager(manager ViewManager)
	RegisterViewRegistry(registry ViewRegistry)
}

type ConfigService = driver.ConfigService

// PostStart enables a platform to execute additional tasks after all platforms have started
type PostStart interface {
	PostStart(context.Context) error
}

type node struct {
	registry      serviceRegistry
	configService ConfigService
	sdks          []api.SDK
	context       context.Context
	cancel        context.CancelFunc
	running       bool
	tracer        trace.Tracer
	viewManager   ViewManager
	viewRegistry  ViewRegistry
}

func NewEmpty(confPath string) *node {
	configService, err := config.NewProvider(confPath)
	if err != nil {
		panic(err)
	}
	registry := registry.New()
	if err := registry.RegisterService(configService); err != nil {
		panic(err)
	}

	return &node{
		registry:      registry,
		sdks:          []api.SDK{},
		configService: configService,
		tracer:        noop.NewTracerProvider().Tracer("noop"),
	}
}

func (n *node) RegisterTracerProvider(provider trace.TracerProvider) {
	logger.Infof("Register tracer provider")
	n.tracer = provider.Tracer("node_view_client", tracing.WithMetricsOpts(tracing.MetricsOpts{
		Namespace:  "viewsdk",
		LabelNames: []tracing.LabelName{fidLabel},
	}))
}

func (n *node) RegisterViewManager(manager ViewManager) {
	n.viewManager = manager
}

func (n *node) RegisterViewRegistry(registry ViewRegistry) {
	n.viewRegistry = registry
}

func (n *node) AddSDK(sdk api.SDK) {
	logger.Infof("Add SDK [%T]", sdk)
	n.sdks = append(n.sdks, sdk)
}

func (n *node) ConfigService() ConfigService {
	return n.configService
}

func (n *node) Start() (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Start triggered panic: %s\n%s\n", r, debug.Stack())
			err = errors.Errorf("Start triggered panic: %s", r)
			n.Stop()
		}
	}()

	n.running = true
	// Install
	logger.Infof("Installing %d sdks...", len(n.sdks))
	for _, p := range n.sdks {
		if err := p.Install(); err != nil {
			logger.Errorf("Failed installing platform [%s]", err)
			return err
		}
	}
	logger.Infof("Installing sdks...done")

	n.context, n.cancel = context.WithCancel(context.Background())

	// Start
	logger.Info("Starting sdks...")
	for _, p := range n.sdks {
		if err := p.Start(n.context); err != nil {
			logger.Errorf("Failed starting platform [%s]", err)
			return err
		}
	}
	logger.Infof("Starting sdks...done")

	// PostStart
	logger.Info("Post-starting sdks...")
	for _, p := range n.sdks {
		ps, ok := p.(PostStart)
		if ok {
			if err := ps.PostStart(n.context); err != nil {
				logger.Errorf("Failed post-starting platform [%s]", err)
				return err
			}
		}
	}
	logger.Infof("Post-starting sdks...done")

	return nil
}

func (n *node) Stop() {
	n.running = false
	if n.cancel != nil {
		n.cancel()
	}
}

func (n *node) InstallSDK(p api.SDK) error {
	if n.running {
		return errors.New("failed installing platform, the system is already running")
	}

	n.sdks = append(n.sdks, p)
	return nil
}

func (n *node) RegisterFactory(id string, factory api.Factory) error {
	return n.viewRegistry.RegisterFactory(id, factory)
}

func (n *node) RegisterResponder(responder view.View, initiatedBy interface{}) error {
	return n.viewRegistry.RegisterResponder(responder, initiatedBy)
}

func (n *node) RegisterResponderWithIdentity(responder view.View, id view.Identity, initiatedBy view.View) error {
	return n.viewRegistry.RegisterResponderWithIdentity(responder, id, initiatedBy)
}

// RegisterService To be deprecated
func (n *node) RegisterService(service interface{}) error {
	return n.registry.RegisterService(service)
}

// GetService to be deprecated
func (n *node) GetService(v interface{}) (interface{}, error) {
	return n.registry.GetService(v)
}

func (n *node) CallView(fid string, in []byte) (interface{}, error) {
	ctx, span := n.tracer.Start(context.Background(), "CallView",
		trace.WithSpanKind(trace.SpanKindClient),
		tracing.WithAttributes(tracing.String(fidLabel, fid)))
	defer span.End()

	span.AddEvent("start_new_view")
	f, err := n.viewManager.NewView(fid, in)
	span.AddEvent("end_new_view")
	if err != nil {
		return nil, errors.Wrapf(err, "failed instantiating view [%s]", fid)
	}
	span.AddEvent("start_initiate_view")
	result, err := n.viewManager.InitiateView(f, ctx)
	span.AddEvent("end_initiate_view")
	if err != nil {
		return nil, errors.Wrapf(err, "failed running view [%s]", fid)
	}
	raw, ok := result.([]byte)
	if !ok {
		raw, err = json.Marshal(result)
		if err != nil {
			return nil, errors.Wrapf(err, "failed marshalling result produced by view %s", fid)
		}
	}
	return raw, nil
}

func (n *node) InitiateContext(view view.View) (view.Context, error) {
	return n.viewManager.InitiateContext(view)
}

// InitiateContextFrom creates a new view context, derived from the passed context.Context
func (n *node) InitiateContextFrom(ctx context.Context, view view.View) (view.Context, error) {
	return n.viewManager.InitiateContextFrom(ctx, view, nil, "")
}

func (n *node) InitiateContextWithIdentity(view view.View, id view.Identity) (view.Context, error) {
	return n.viewManager.InitiateContextWithIdentity(view, id)
}

func (n *node) Context(contextID string) (view.Context, error) {
	return n.viewManager.Context(contextID)
}

func (n *node) Initiate(fid string, in []byte) (string, error) {
	panic("implement me")
}
