/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

type EndpointService interface {
	Resolve(party view2.Identity) (view2.Identity, map[view.PortName]string, []byte, error)
	GetIdentity(label string, pkID []byte) (view2.Identity, error)
}

type ConfigService interface {
	GetString(key string) string
	GetPath(key string) string
}

type Service struct {
	HostProvider    host.GeneratorProvider
	EndpointService EndpointService
	ConfigService   ConfigService
	DefaultIdentity view2.Identity

	Node            *P2PNode
	NodeSync        sync.RWMutex
	tracerProvider  trace.TracerProvider
	metricsProvider metrics.Provider
}

func NewService(hostProvider host.GeneratorProvider, endpointService EndpointService, configService ConfigService, defaultIdentity view2.Identity, tracerProvider trace.TracerProvider, metricsProvider metrics.Provider) (*Service, error) {
	s := &Service{
		HostProvider:    hostProvider,
		EndpointService: endpointService,
		ConfigService:   configService,
		DefaultIdentity: defaultIdentity,
		tracerProvider:  tracerProvider,
		metricsProvider: metricsProvider,
	}
	return s, nil
}

func (s *Service) Start(ctx context.Context) {
	go func() {
		for {
			logger.Infof("start communication service...")
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

func (s *Service) NewSessionWithID(sessionID, contextID, endpoint string, pkid []byte, caller view2.Identity, msg *view2.Message) (view2.Session, error) {
	if err := s.init(); err != nil {
		return nil, errors.Errorf("communication service not ready [%s]", err)
	}
	return s.Node.NewSessionWithID(sessionID, contextID, endpoint, pkid, caller, msg)
}

func (s *Service) NewSession(caller string, contextID string, endpoint string, pkid []byte) (view2.Session, error) {
	if err := s.init(); err != nil {
		return nil, errors.Errorf("communication service not ready [%s]", err)
	}
	return s.Node.NewSession(caller, contextID, endpoint, pkid)
}

func (s *Service) MasterSession() (view2.Session, error) {
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

func (s *Service) Addresses(id view2.Identity) ([]string, error) {
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

	p2pListenAddress := s.ConfigService.GetString("fsc.p2p.listenAddress")
	p2pBootstrapNode := s.ConfigService.GetString("fsc.p2p.opts.bootstrapNode")
	keyFile := s.ConfigService.GetPath("fsc.identity.key.file")
	certFile := s.ConfigService.GetPath("fsc.identity.cert.file")
	if len(p2pBootstrapNode) == 0 {
		// this is a bootstrap node
		logger.Infof("new p2p bootstrap node [%s]", p2pListenAddress)

		h, err := s.HostProvider.NewBootstrapHost(p2pListenAddress, keyFile, certFile)
		if err != nil {
			return err
		}
		s.Node, err = NewNode(h, s.tracerProvider, s.metricsProvider)
		if err != nil {
			return errors.Wrapf(err, "failed to initialize bootstrap p2p node [%s]", p2pListenAddress)
		}
	} else {
		bootstrapNodeID, err := s.EndpointService.GetIdentity(p2pBootstrapNode, nil)
		if err != nil {
			return errors.WithMessagef(err, "failed to get p2p bootstrap node's resolver entry [%s]", p2pBootstrapNode)
		}
		_, endpoints, pkID, err := s.EndpointService.Resolve(bootstrapNodeID)
		if err != nil {
			return errors.WithMessagef(err, "failed to resolve bootstrap node id [%s:%s]", p2pBootstrapNode, bootstrapNodeID)
		}

		addr, err := utils.AddressToEndpoint(endpoints[view.P2PPort])
		if err != nil {
			return errors.WithMessagef(err, "failed to get the endpoint of the bootstrap node from [%s:%s], [%s]", p2pBootstrapNode, bootstrapNodeID, endpoints[view.P2PPort])
		}
		addr = addr + "/p2p/" + string(pkID)
		logger.Infof("new p2p node [%s,%s]", p2pListenAddress, addr)
		h, err := s.HostProvider.NewHost(p2pListenAddress, keyFile, certFile, addr)
		if err != nil {
			return err
		}
		s.Node, err = NewNode(h, s.tracerProvider, s.metricsProvider)
		if err != nil {
			return errors.Wrapf(err, "failed to initialize node p2p manager [%s,%s]", p2pListenAddress, addr)
		}
	}
	return nil
}
