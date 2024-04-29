/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"context"
	"encoding/json"
	"log"
	"reflect"
	"runtime/debug"

	config2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/core/config"
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/api"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	view3 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	viewsdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	registry2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/registry"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var logger = flogging.MustGetLogger("fsc")

type ExecuteCallbackFunc = func() error

type ViewManager interface {
	NewView(id string, in []byte) (view.View, error)
	InitiateView(view view.View) (interface{}, error)
	InitiateContext(view view.View) (view.Context, error)
	InitiateContextWithIdentity(view view.View, id view.Identity) (view.Context, error)
	Context(contextID string) (view.Context, error)
}

type Registry interface {
	GetService(v interface{}) (interface{}, error)

	RegisterService(service interface{}) error
}

type ConfigService interface {
	GetString(key string) string
}

// PostStart enables a platform to execute additional tasks after all platforms have started
type PostStart interface {
	PostStart(context.Context) error
}

type node struct {
	registry      Registry
	configService ConfigService
	sdks          []api.SDK
	context       context.Context
	cancel        context.CancelFunc
	running       bool
}

func New() *node {
	return NewFromConfPath("")
}

func NewFromConfPath(confPath string) *node {
	registry := registry2.New()
	configService, err := config2.NewProvider(confPath)
	if err != nil {
		panic(err)
	}
	platforms := []api.SDK{
		viewsdk.NewSDK(configService, registry),
	}

	node := &node{
		sdks:          platforms,
		registry:      registry,
		configService: configService,
	}

	return node
}

func NewEmpty(confPath string) *node {
	configService, err := config2.NewProvider(confPath)
	if err != nil {
		panic(err)
	}
	return &node{
		sdks:          []api.SDK{},
		registry:      registry2.New(),
		configService: configService,
	}
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
	logger.Infof("Installing sdks...")
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
	return view3.GetRegistry(n.registry).RegisterFactory(id, factory)
}

func (n *node) RegisterResponder(responder view.View, initiatedBy interface{}) error {
	return view3.GetRegistry(n.registry).RegisterResponder(responder, initiatedBy)
}

func (n *node) RegisterResponderWithIdentity(responder view.View, id view.Identity, initiatedBy view.View) error {
	return view3.GetRegistry(n.registry).RegisterResponderWithIdentity(responder, id, initiatedBy)
}

func (n *node) RegisterService(service interface{}) error {
	return n.registry.RegisterService(service)
}

func (n *node) GetService(v interface{}) (interface{}, error) {
	return n.registry.GetService(v)
}

func (n *node) Registry() Registry {
	return n.registry
}

func (n *node) ResolveIdentities(endpoints ...string) ([]view.Identity, error) {
	resolver := view3.GetEndpointService(n.registry)

	var ids []view.Identity
	for _, e := range endpoints {
		identity, err := resolver.GetIdentity(e, nil)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot find the identity at %s", e)
		}
		ids = append(ids, identity)
	}

	return ids, nil
}

func (n *node) IsTxFinal(txID string, opts ...api.ServiceOption) error {
	options, err := api.CompileServiceOptions(opts...)
	if err != nil {
		return errors.Wrapf(err, "failed to compile service options")
	}
	c := context.Background()
	if options.Timeout != 0 {
		var cancel context.CancelFunc
		c, cancel = context.WithTimeout(c, options.Timeout)
		defer cancel()
	}
	// TODO: network might refer to orion
	_, ch, err := fabric.GetChannel(n.registry, options.Network, options.Channel)
	if err != nil {
		return err
	}
	return ch.Finality().IsFinal(c, txID)
}

func (n *node) CallView(fid string, in []byte) (interface{}, error) {
	s, err := n.GetService(reflect.TypeOf((*ViewManager)(nil)))
	if err != nil {
		return nil, err
	}
	manager := s.(ViewManager)

	f, err := manager.NewView(fid, in)
	if err != nil {
		return nil, errors.Wrapf(err, "failed instantiating view [%s]", fid)
	}
	result, err := manager.InitiateView(f)
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
	s, err := n.GetService(reflect.TypeOf((*ViewManager)(nil)))
	if err != nil {
		return nil, err
	}
	manager := s.(ViewManager)

	return manager.InitiateContext(view)
}

func (n *node) InitiateContextWithIdentity(view view.View, id view.Identity) (view.Context, error) {
	s, err := n.GetService(reflect.TypeOf((*ViewManager)(nil)))
	if err != nil {
		return nil, err
	}
	manager := s.(ViewManager)

	return manager.InitiateContextWithIdentity(view, id)
}

func (n *node) Context(contextID string) (view.Context, error) {
	s, err := n.GetService(reflect.TypeOf((*ViewManager)(nil)))
	if err != nil {
		return nil, err
	}
	manager := s.(ViewManager)

	return manager.Context(contextID)
}

func (n *node) Initiate(fid string, in []byte) (string, error) {
	panic("implement me")
}

func (n *node) Track(cid string) string {
	panic("implement me")
}
