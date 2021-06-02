/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package comm

import (
	"context"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type EndpointService interface {
	Resolve(party view2.Identity) (view2.Identity, map[view.PortName]string, []byte, error)
	GetIdentity(label string, pkID []byte) (view2.Identity, error)
}

type ConfigService interface {
	GetString(key string) string
}

type Service struct {
	PrivateKeyDispenser PrivateKeyDispenser
	EndpointService     EndpointService
	ConfigService       ConfigService
	DefaultIdentity     view2.Identity
	Node                *P2PNode
}

func NewService(
	privateKeyDispenser PrivateKeyDispenser,
	endpointService EndpointService,
	configService ConfigService,
	defaultIdentity view2.Identity,
) (*Service, error) {
	s := &Service{
		PrivateKeyDispenser: privateKeyDispenser,
		EndpointService:     endpointService,
		ConfigService:       configService,
		DefaultIdentity:     defaultIdentity,
	}
	if err := s.init(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) Start(ctx context.Context) {
	s.Node.Start(ctx)
}

func (s *Service) Stop() {
	s.Node.Stop()
}

func (s *Service) NewSessionWithID(sessionID, contextID, endpoint string, pkid []byte, caller view2.Identity, msg *view2.Message) (view2.Session, error) {
	return s.Node.NewSessionWithID(sessionID, contextID, endpoint, pkid, caller, msg)
}

func (s *Service) NewSession(caller string, contextID string, endpoint string, pkid []byte) (view2.Session, error) {
	return s.Node.NewSession(caller, contextID, endpoint, pkid)
}

func (s *Service) MasterSession() (view2.Session, error) {
	return s.Node.MasterSession()
}

func (s *Service) init() error {
	p2pListenAddress := s.ConfigService.GetString("fsc.p2p.listenAddress")
	p2pBootstrapNode := s.ConfigService.GetString("fsc.p2p.bootstrapNode")
	if len(p2pBootstrapNode) == 0 {
		// this is a bootstrap node
		_, endpoint, pkid, err := s.EndpointService.Resolve(s.DefaultIdentity)
		if err != nil {
			return errors.Wrapf(err, "failed resolving bootstrap p2p identity [%s]", p2pListenAddress)
		}
		logger.Infof("new p2p bootstrap node [%s,%s,%s]", p2pListenAddress, endpoint, pkid)

		s.Node, err = NewBootstrapNode(
			p2pListenAddress,
			s.PrivateKeyDispenser,
		)
		if err != nil {
			return errors.Wrapf(err, "failed initializing bootstrap p2p manager [%s,%s,%s]", p2pListenAddress, endpoint, pkid)
		}
	} else {
		bootstrapNodeID, err := s.EndpointService.GetIdentity(p2pBootstrapNode, nil)
		if err != nil {
			return err
		}
		_, endpoints, pkID, err := s.EndpointService.Resolve(bootstrapNodeID)
		if err != nil {
			return err
		}

		logger.Infof("new p2p node [%s,%s]", p2pListenAddress, AddressToEndpoint(endpoints[view.P2PPort])+"/p2p/"+string(pkID))
		s.Node, err = NewNode(
			p2pListenAddress,
			AddressToEndpoint(endpoints[view.P2PPort])+"/p2p/"+string(pkID),
			s.PrivateKeyDispenser,
		)
		if err != nil {
			return errors.Wrapf(err, "failed initializing node p2p manager [%s,%s]", p2pListenAddress, AddressToEndpoint(endpoints[view.P2PPort])+"/p2p/"+string(pkID))
		}
	}
	return nil
}
