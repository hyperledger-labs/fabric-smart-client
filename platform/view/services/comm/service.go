/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var (
	ErrNotInitialized = errors.New("communication service not initialized")
	ErrNotStarted     = errors.New("communication service not started")
)

type EndpointService interface {
	GetIdentity(label string, pkID []byte) (view.Identity, error)
}

type ConfigService interface {
	GetString(key string) string
	GetPath(key string) string
	GetInt(key string) int
	IsSet(key string) bool
}

type Service struct {
	HostProvider    host.GeneratorProvider
	EndpointService EndpointService
	ConfigService   ConfigService

	Node            *P2PNode
	NodeSync        sync.RWMutex
	metricsProvider metrics.Provider
	initialized     atomic.Bool
	ctx             context.Context
}

func NewService(hostProvider host.GeneratorProvider, endpointService EndpointService, configService ConfigService, metricsProvider metrics.Provider) (*Service, error) {
	s := &Service{
		HostProvider:    hostProvider,
		EndpointService: endpointService,
		ConfigService:   configService,
		metricsProvider: metricsProvider,
	}
	return s, nil
}

func (s *Service) Start(ctx context.Context) {
	s.NodeSync.Lock()
	s.ctx = ctx
	s.NodeSync.Unlock()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				logger.Infof("init communication service...")
				if err := s.init(); err != nil {
					logger.Errorf("failed to initialize communication service [%s], wait a bit and try again", err)
					select {
					case <-time.After(10 * time.Second):
						continue
					case <-ctx.Done():
						return
					}
				}
				// Init done, we can start
				logger.Infof("start communication service...")
				s.Node.Start(ctx)
				return
			}
		}
	}()
}

func (s *Service) Stop() {
	if !s.initialized.Load() {
		logger.Warnf("%s, cannot stop", ErrNotInitialized)
		return
	}
	s.Node.Stop()
}

func (s *Service) MasterSession() (view.Session, error) {
	if err := s.init(); err != nil {
		return nil, ErrNotInitialized
	}
	return s.Node.MasterSession()
}

func (s *Service) NewResponderSession(caller []byte, msg *view.Message) (view.Session, error) {
	if err := s.init(); err != nil {
		return nil, ErrNotInitialized
	}

	return s.Node.NewResponderSession(
		msg.SessionID,
		msg.ContextID,
		msg.FromEndpoint,
		msg.FromPKID,
		caller,
		msg,
	)
}

func (s *Service) NewSessionWithID(sessionID, contextID, endpoint string, pkid []byte) (view.Session, error) {
	if err := s.init(); err != nil {
		return nil, ErrNotInitialized
	}

	return s.Node.NewSessionWithID(sessionID, contextID, endpoint, pkid)
}

func (s *Service) NewSession(caller string, contextID string, endpoint string, pkid []byte) (view.Session, error) {
	if err := s.init(); err != nil {
		return nil, ErrNotInitialized
	}

	return s.Node.NewSession(caller, contextID, endpoint, pkid)
}

func (s *Service) DeleteSessions(ctx context.Context, sessionID string) {
	if err := s.init(); err != nil {
		logger.Warnf("%s, cannot delete any session", ErrNotInitialized)
		return
	}
	s.Node.DeleteSessions(ctx, sessionID)
}

func (s *Service) init() error {
	s.NodeSync.RLock()
	if s.Node != nil {
		s.NodeSync.RUnlock()
		return nil
	}
	s.NodeSync.RUnlock()
	s.NodeSync.Lock()
	defer s.NodeSync.Unlock()
	if s.Node != nil {
		return nil
	}

	if s.ctx == nil {
		return ErrNotStarted
	}

	h, err := s.HostProvider.GetNewHost()
	if err != nil {
		return err
	}

	cfg := NewConfig(s.ConfigService)

	s.Node, err = NewNodeWithConfig(s.ctx, h, s.metricsProvider, cfg)
	if err != nil {
		return err
	}
	s.initialized.Store(true)
	return nil
}
