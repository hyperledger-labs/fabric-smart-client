/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package endpoint

import (
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/generic/core/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

const (
	bccspMSP = "bccsp"
)

var logger = flogging.MustGetLogger("fabric-sdk.endpoint")

type IdentityConf struct {
	ID   string `yaml:"id"`
	Path string `yaml:"path"`
}

type resolver struct {
	Name           string            `yaml:"name,omitempty"`
	Domain         string            `yaml:"domain,omitempty"`
	Identity       IdentityConf      `yaml:"identity,omitempty"`
	Addresses      map[string]string `yaml:"addresses,omitempty"`
	Aliases        []string          `yaml:"aliases,omitempty"`
	Id             []byte
	IdentityGetter func() (view.Identity, error)
}

func (r *resolver) GetIdentity() (view.Identity, error) {
	if r.IdentityGetter != nil {
		return r.IdentityGetter()
	}

	return r.Id, nil
}

type PKIResolver interface {
	GetPKIidOfCert(peerIdentity view.Identity) []byte
}

type ConfigService interface {
	IsSet(s string) bool
	UnmarshalKey(s string, i interface{}) error
	TranslatePath(path string) string
}

type Service interface {
	Bind(longTerm view.Identity, ephemeral view.Identity) error
	AddResolver(name string, domain string, addresses map[string]string, aliases []string, id []byte) error
	AddPKIResolver(resolver PKIResolver) error
}

type resolverService struct {
	config  ConfigService
	service Service
}

// NewResolverService returns a new instance of the view-sdk endpoint resolverService
func NewResolverService(config ConfigService, service Service) (*resolverService, error) {
	er := &resolverService{
		config:  config,
		service: service,
	}
	if err := service.AddPKIResolver(NewPKIResolver()); err != nil {
		return nil, errors.Wrapf(err, "failed adding fabric pki resolver")
	}
	return er, nil
}

func (r *resolverService) LoadResolvers() error {
	// Load entry
	if r.config.IsSet("fabric.endpoint.resolves") {
		logger.Infof("loading resolvers")
		var resolvers []*resolver
		err := r.config.UnmarshalKey("fabric.endpoint.resolves", &resolvers)
		if err != nil {
			logger.Errorf("failed loading resolves [%s]", err)
			return err
		}
		logger.Infof("loaded resolves successfully, number of entries found %d", len(resolvers))

		for _, resolver := range resolvers {
			// Load identity
			raw, err := id.Serialize(r.config.TranslatePath(resolver.Identity.Path))
			if err != nil {
				return err
			}
			resolver.Id = raw
			logger.Infof("resolver [%s,%s][%s] %s",
				resolver.Name, resolver.Domain, resolver.Addresses,
				view.Identity(resolver.Id).UniqueID(),
			)

			// Add entry
			if err := r.service.AddResolver(resolver.Name, resolver.Domain, resolver.Addresses, resolver.Aliases, resolver.Id); err != nil {
				return errors.Wrapf(err, "failed adding resolver")
			}

			// Bind Aliases
			for _, alias := range resolver.Aliases {
				logger.Debugf("binging [%s] to [%s]", resolver.Name, alias)
				if err := r.service.Bind(resolver.Id, []byte(alias)); err != nil {
					return errors.WithMessagef(err, "failed binding identity [%s] to alias [%s]", resolver.Name, alias)
				}
			}
		}
	}
	return nil
}
