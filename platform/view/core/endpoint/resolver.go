/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"

	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type MspConf struct {
	ID      string `yaml:"id"`
	MSPType string `yaml:"mspType"`
	MSPID   string `yaml:"mspID"`
	Path    string `yaml:"path"`
}

type entry struct {
	Name           string            `yaml:"name,omitempty"`
	Domain         string            `yaml:"domain,omitempty"`
	Identity       MspConf           `yaml:"identity,omitempty"`
	Addresses      map[string]string `yaml:"addresses,omitempty"`
	Aliases        []string          `yaml:"aliases,omitempty"`
	Id             []byte
	IdentityGetter func() (view.Identity, []byte, error)
}

func (r *entry) GetIdentity() (view.Identity, error) {
	if r.IdentityGetter != nil {
		id, _, err := r.IdentityGetter()
		return id, err
	}

	return r.Id, nil
}

type ConfigService interface {
	GetString(key string) string
	IsSet(s string) bool
	UnmarshalKey(s string, i interface{}) error
	TranslatePath(path string) string
}

type IdentityService interface {
	DefaultIdentity() view.Identity
}

type Service interface {
	Bind(longTerm view.Identity, ephemeral view.Identity) error
	AddResolver(name string, domain string, addresses map[string]string, aliases []string, id []byte) (view.Identity, error)
	AddPKIResolver(resolver view2.PKIResolver) error
}

type resolverService struct {
	config  ConfigService
	service Service
	is      IdentityService
}

// NewResolverService returns a new instance of the view-sdk endpoint resolverService
func NewResolverService(config ConfigService, service Service, is IdentityService) (*resolverService, error) {
	er := &resolverService{
		config:  config,
		service: service,
		is:      is,
	}
	if err := service.AddPKIResolver(NewPKIResolver()); err != nil {
		return nil, errors.Wrapf(err, "failed adding fabric pki resolver")
	}
	return er, nil
}

func (r *resolverService) LoadResolvers() error {
	// Load entry
	if r.config.IsSet("fsc.endpoint.resolvers") {
		logger.Infof("loading resolvers")
		var resolvers []*entry
		err := r.config.UnmarshalKey("fsc.endpoint.resolvers", &resolvers)
		if err != nil {
			logger.Errorf("failed loading resolvers [%s]", err)
			return errors.Wrapf(err, "failed loading resolvers")
		}
		logger.Infof("loaded resolvers successfully, number of entries found %d", len(resolvers))

		// add default
		_, err = r.service.AddResolver(
			r.config.GetString("fsc.id"),
			"",
			map[string]string{
				string(driver.ViewPort): r.config.GetString("fsc.address"),
			},
			nil,
			r.is.DefaultIdentity(),
		)
		if err != nil {
			logger.Errorf("failed adding default resolver [%s]", err)
			return errors.Wrapf(err, "failed adding default resolver")
		}

		for _, resolver := range resolvers {
			// Load identity
			raw, err := id.LoadIdentity(r.config.TranslatePath(resolver.Identity.Path))
			if err != nil {
				return err
			}
			resolver.Id = raw
			logger.Infof("resolver [%s,%s][%s] %s",
				resolver.Name, resolver.Domain, resolver.Addresses,
				view.Identity(resolver.Id).UniqueID(),
			)

			// Add entry
			if _, err := r.service.AddResolver(resolver.Name, resolver.Domain, resolver.Addresses, resolver.Aliases, resolver.Id); err != nil {
				return errors.Wrapf(err, "failed adding resolver")
			}

			// Bind Aliases
			for _, alias := range resolver.Aliases {
				if logger.IsEnabledFor(zapcore.DebugLevel) {
					logger.Debugf("binging [%s] to [%s]", resolver.Name, alias)
				}
				if err := r.service.Bind(resolver.Id, []byte(alias)); err != nil {
					return errors.WithMessagef(err, "failed binding identity [%s] to alias [%s]", resolver.Name, alias)
				}
			}
		}
	}
	return nil
}
