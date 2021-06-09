/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package endpoint

import (
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/x509"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

const (
	bccspMSP = "bccsp"
)

var logger = flogging.MustGetLogger("fabric-sdk.endpoint")

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
	RootID         view.Identity
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
	IsSet(s string) bool
	UnmarshalKey(s string, i interface{}) error
	TranslatePath(path string) string
}

type Service interface {
	Bind(longTerm view.Identity, ephemeral view.Identity) error
	AddResolver(name string, domain string, addresses map[string]string, aliases []string, id []byte) (view.Identity, error)
	AddPKIResolver(resolver view2.PKIResolver) error
}

type resolverService struct {
	config    ConfigService
	service   Service
	resolvers []*entry
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
		var resolvers []*entry
		err := r.config.UnmarshalKey("fabric.endpoint.resolves", &resolvers)
		if err != nil {
			logger.Errorf("failed loading resolves [%s]", err)
			return err
		}
		logger.Infof("loaded resolves successfully, number of entries found %d", len(resolvers))

		for _, resolver := range resolvers {
			// Load identity
			var raw []byte
			switch resolver.Identity.MSPType {
			case bccspMSP:
				raw, err = x509.Serialize(resolver.Identity.MSPID, r.config.TranslatePath(resolver.Identity.Path))
				if err != nil {
					return errors.Wrapf(err, "failed serializing x509 identity")
				}
			default:
				return errors.Errorf("expected bccsp type, got %s", resolver.Identity.MSPType)
			}
			resolver.Id = raw
			logger.Debugf("entry [%s,%s][%s] %s",
				resolver.Name, resolver.Domain, resolver.Addresses,
				view.Identity(resolver.Id).UniqueID(),
			)

			// Add entry
			rootID, err := r.service.AddResolver(resolver.Name, resolver.Domain, resolver.Addresses, resolver.Aliases, resolver.Id)
			if err != nil {
				return errors.Wrapf(err, "failed adding resolver")
			}
			logger.Debugf("added resolver [root-id:%s]", rootID.String())
			resolver.RootID = rootID

			// Bind Aliases
			for _, alias := range resolver.Aliases {
				logger.Debugf("binging [%s] to [%s]", resolver.Name, alias)
				if err := r.service.Bind(resolver.Id, []byte(alias)); err != nil {
					return errors.WithMessagef(err, "failed binding identity [%s] to alias [%s]", resolver.Name, alias)
				}
			}
		}
		r.resolvers = resolvers
	}
	return nil
}

func (r *resolverService) GetIdentity(label string) view.Identity {
	for _, resolver := range r.resolvers {
		if resolver.Name == label {
			return resolver.Id
		}
		for _, alias := range resolver.Aliases {
			if alias == label {
				return resolver.Id
			}
		}
		if view.Identity(resolver.Id).UniqueID() == label {
			return resolver.Id
		}
		if resolver.RootID.UniqueID() == label {
			return resolver.Id
		}
	}
	return nil
}
