/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"context"
	"runtime/debug"
	"slices"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	utils "github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
)

var logger = logging.MustGetLogger()

// PostStart enables a platform to execute additional tasks after all platforms have started
type PostStart interface {
	PostStart(context.Context) error
}

// stopper is implemented by an SDK that releases what it started, such as every
// commondig.SDK. Node.Stop calls Stop on every SDK that implements it, in reverse install
// order, after the node context is cancelled.
type stopper interface {
	Stop() error
}

// Drainer is a service, registered in the node's registry, that lets running work finish
// before the node stops; the view SDK registers the view manager. Node.Stop calls Drain on the
// first Drainer the registry returns before it cancels the node context. Drain makes the node
// refuse new work and returns once running work has finished. When ctx is done first, it
// returns promptly, even if some work ignores cancellation.
type Drainer interface {
	Drain(ctx context.Context) error
}

const (
	// drainTimeoutKey bounds how long Node.Stop waits for running views to finish.
	drainTimeoutKey = "fsc.shutdown.drainTimeout"
	// teardownTimeoutKey bounds how long Node.Stop waits for the SDKs to stop.
	teardownTimeoutKey = "fsc.shutdown.teardownTimeout"
	// defaultShutdownTimeout applies to both keys when they are unset or negative.
	defaultShutdownTimeout = 30 * time.Second
)

// configService is the configuration Node.Stop reads its timeouts from.
type configService interface {
	IsSet(key string) bool
	GetDuration(key string) time.Duration
}

// drainTimeout returns fsc.shutdown.drainTimeout: defaultShutdownTimeout when it is unset or
// negative, and 0, which does not wait, when it is 0.
func drainTimeout(c configService) time.Duration { return shutdownTimeout(c, drainTimeoutKey) }

// teardownTimeout returns fsc.shutdown.teardownTimeout, read as drainTimeout reads its key.
func teardownTimeout(c configService) time.Duration { return shutdownTimeout(c, teardownTimeoutKey) }

func shutdownTimeout(c configService, key string) time.Duration {
	if c == nil || !c.IsSet(key) {
		return defaultShutdownTimeout
	}
	if d := c.GetDuration(key); d >= 0 {
		return d
	}
	return defaultShutdownTimeout
}

// Node is a struct that allows the developer to compose sequentially multiple SDKs.
// The struct offers also a service locator that the SDKs can use to register services.
type Node struct {
	registry utils.Registry
	config   configService
	id       string
	sdks     []SDK
	ctx      context.Context
	cancel   context.CancelFunc
	running  bool

	// stopMu guards stopped, drainCtx and skipDrain, which the first Stop call sets. stopped is
	// closed when the shutdown has finished; skipDrain ends drainCtx, the drain's context.
	stopMu    sync.Mutex
	stopped   chan struct{}
	drainCtx  context.Context
	skipDrain context.CancelFunc
}

func NewFromConfPath(confPath string) *Node {
	n, err := NewFromConfPathE(confPath)
	if err != nil {
		panic(err)
	}
	return n
}

// NewFromConfPathE returns a new node for the given configuration path or an error.
func NewFromConfPathE(confPath string) (*Node, error) {
	cfg, err := config.NewProvider(confPath)
	if err != nil {
		return nil, err
	}
	registry := view.NewServiceProvider()
	if err := registry.RegisterService(cfg); err != nil {
		return nil, err
	}

	return &Node{
		sdks:     []SDK{},
		registry: registry,
		config:   cfg,
		id:       cfg.ID(),
	}, nil
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
			logger.Errorf("start triggered panic: %s\n%s", r, debug.Stack())
			err = errors.Errorf("start triggered panic: %s", r)
			n.Stop()
		}
	}()

	// Install
	logger.Infof("installing sdks...")
	for _, p := range n.sdks {
		if err := p.Install(); err != nil {
			logger.Errorf("failed installing platform [%s]", err.Error())
			return err
		}
	}
	logger.Infof("installing sdks...done")

	n.ctx, n.cancel = context.WithCancel(context.Background())

	// Start
	logger.Info("starting sdks...")
	for _, p := range n.sdks {
		if err := p.Start(n.ctx); err != nil {
			logger.Errorf("failed starting platform [%s]", err.Error())
			return err
		}
	}
	logger.Infof("starting sdks...done")

	// PostStart
	logger.Info("post-starting sdks...")
	for _, p := range n.sdks {
		ps, ok := p.(PostStart)
		if ok {
			if err := ps.PostStart(n.ctx); err != nil {
				logger.Errorf("failed post-starting platform [%s]", err.Error())
				return err
			}
		}
	}
	logger.Infof("Post-starting sdks...done")

	n.running = true
	return nil
}

// Stop shuts the node down and returns once it has stopped:
//  1. it drains: the Drainer in the registry refuses new work and waits for running views, for
//     at most fsc.shutdown.drainTimeout;
//  2. it cancels the node context passed to Start;
//  3. it calls Stop on every SDK that has a Stop() error method, in reverse install order,
//     for at most fsc.shutdown.teardownTimeout in total. SDKs still stopping at the timeout are
//     logged and left running.
//
// A call while a shutdown runs skips the rest of its drain and waits for it to finish. Stop is
// safe to call on a node that was never started or failed to start.
func (n *Node) Stop() {
	n.stopMu.Lock()
	first := n.stopped == nil
	if first {
		n.stopped = make(chan struct{})
		n.drainCtx, n.skipDrain = context.WithTimeout(context.Background(), drainTimeout(n.config))
	} else {
		logger.Infof("shutdown in progress, skipping the rest of the drain")
		n.skipDrain()
	}
	stopped := n.stopped
	n.stopMu.Unlock()

	if !first {
		<-stopped
		return
	}
	defer close(stopped)
	start := time.Now()
	n.running = false

	n.drain(n.drainCtx)
	n.skipDrain()

	if n.cancel != nil {
		n.cancel()
	}
	n.teardown(teardownTimeout(n.config))
	logger.Infof("node stopped in %s", time.Since(start))
}

// drain runs the Drainer registered in the registry, if any, and logs what still runs every
// 5s until it returns.
func (n *Node) drain(ctx context.Context) {
	if n.registry == nil {
		return
	}
	s, err := n.registry.GetService(new(Drainer))
	if err != nil {
		return
	}
	d, ok := s.(Drainer)
	if !ok {
		return
	}
	logger.Infof("draining running views...")
	done := make(chan error, 1)
	go func() { done <- d.Drain(ctx) }()
	ticker := time.NewTicker(drainLogInterval)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			if err != nil {
				logger.Warnf("draining running views...stopped before they finished: %s", err)
				return
			}
			logger.Infof("draining running views...done")
			return
		case <-ticker.C:
			logger.Infof("draining running views...still waiting")
		}
	}
}

// drainLogInterval is how often drain logs that it is still waiting.
const drainLogInterval = 5 * time.Second

// teardown stops the SDKs in reverse install order, within timeout.
func (n *Node) teardown(timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		for _, v := range slices.Backward(n.sdks) {
			s, ok := v.(stopper)
			if !ok {
				continue
			}
			if err := s.Stop(); err != nil {
				logger.Errorf("failed stopping sdk [%T]: %s", v, err)
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		logger.Warnf("sdks did not stop within %s", timeout)
	}
}

func (n *Node) InstallSDK(p SDK) error {
	if n.running {
		return errors.New("failed installing platform, the system is already running")
	}

	n.sdks = append(n.sdks, p)
	return nil
}

func (n *Node) RegisterService(service any) error {
	return n.registry.RegisterService(service)
}

func (n *Node) GetService(v any) (any, error) {
	return n.registry.GetService(v)
}
