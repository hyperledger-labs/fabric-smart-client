/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type EndpointService interface {
	GetIdentity(label string, pkID []byte) (view.Identity, error)
}

type ConfigService interface {
	GetString(key string) string
	GetPath(key string) string
}

type Service struct {
	HostProvider    host.GeneratorProvider
	EndpointService EndpointService
	ConfigService   ConfigService

	Node            *P2PNode
	NodeSync        sync.RWMutex
	metricsProvider metrics.Provider
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
	go func() {
		for {
			logger.Debugf("start communication service...")
			if err := s.init(); err != nil {
				logger.Errorf("failed to initialize communication service [%s], wait a bit and try again", err)
				time.Sleep(10 * time.Second)
				continue
			}
			// Init done, we can start
			s.Node.Start(ctx)
			break
		}
	}()
}

func (s *Service) Stop() {
	if err := s.init(); err != nil {
		logger.Warnf("communication service not ready [%s], cannot stop", err)
		return
	}
	s.Node.Stop()
}

func (s *Service) NewSessionWithID(sessionID, contextID, endpoint string, pkid []byte, caller view.Identity, msg *view.Message) (view.Session, error) {
	if err := s.init(); err != nil {
		return nil, errors.Errorf("communication service not ready [%s]", err)
	}
	return s.Node.NewSessionWithID(sessionID, contextID, endpoint, pkid, caller, msg)
}

func (s *Service) NewSession(caller string, contextID string, endpoint string, pkid []byte) (view.Session, error) {
	if err := s.init(); err != nil {
		return nil, errors.Errorf("communication service not ready [%s]", err)
	}
	return s.Node.NewSession(caller, contextID, endpoint, pkid)
}

func (s *Service) MasterSession() (view.Session, error) {
	if err := s.init(); err != nil {
		return nil, errors.Errorf("communication service not ready [%s]", err)
	}
	return s.Node.MasterSession()
}

func (s *Service) DeleteSessions(ctx context.Context, sessionID string) {
	if err := s.init(); err != nil {
		logger.Warnf("communication service not ready [%s], cannot delete any session", err)
		return
	}
	s.Node.DeleteSessions(ctx, sessionID)
}

func (s *Service) Addresses(id view.Identity) ([]string, error) {
	// TODO: implement this
	return nil, nil
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

	h, err := s.HostProvider.GetNewHost()
	if err != nil {
		return err
	}
	s.Node, err = NewNode(h, s.metricsProvider)
	return err
}
