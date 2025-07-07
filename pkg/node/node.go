/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"context"
	"log"
	"runtime/debug"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	utils "github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/pkg/errors"
)

var logger = logging.MustGetLogger()

type ExecuteCallbackFunc = func() error

// PostStart enables a platform to execute additional tasks after all platforms have started
type PostStart interface {
	PostStart(context.Context) error
}

// Node is a struct that allows the developer to compose sequentially multiple SDKs.
// The struct offers also a service locator that the SDKs can use to register services.
type Node struct {
	registry utils.Registry
	id       string
	sdks     []SDK
	context  context.Context
	cancel   context.CancelFunc
	running  bool
}

func NewFromConfPath(confPath string) *Node {
	configService, err := config.NewProvider(confPath)
	if err != nil {
		panic(err)
	}
	registry := view.New()
	if err := registry.RegisterService(configService); err != nil {
		panic(err)
	}

	return &Node{
		sdks:     []SDK{},
		registry: registry,
		id:       configService.ID(),
	}
}

func (n *Node) AddSDK(sdk SDK) {
	n.sdks = append(n.sdks, sdk)
}

func (n *Node) ID() string {
	return n.id
}

func (n *Node) Start() (err error) {
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

func (n *Node) Stop() {
	n.running = false
	if n.cancel != nil {
		n.cancel()
	}
}

func (n *Node) InstallSDK(p SDK) error {
	if n.running {
		return errors.New("failed installing platform, the system is already running")
	}

	n.sdks = append(n.sdks, p)
	return nil
}

func (n *Node) RegisterService(service interface{}) error {
	return n.registry.RegisterService(service)
}

func (n *Node) GetService(v interface{}) (interface{}, error) {
	return n.registry.GetService(v)
}
