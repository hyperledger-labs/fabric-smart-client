/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint

import (
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/x509"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

const (
	bccspMSP = "bccsp"
)

var logger = flogging.MustGetLogger("fabric-sdk.endpoint")

type Resolver struct {
	config.Resolver
	Id             []byte
	RootID         view.Identity
	IdentityGetter func() (view.Identity, []byte, error)
}

func (r *Resolver) GetIdentity() (view.Identity, error) {
	if r.IdentityGetter != nil {
		id, _, err := r.IdentityGetter()
		return id, err
	}

	return r.Id, nil
}

type Service interface {
	Bind(longTerm view.Identity, ephemeral view.Identity) error
	AddResolver(name string, domain string, addresses map[string]string, aliases []string, id []byte) (view.Identity, error)
	AddPKIResolver(resolver view2.PKIResolver) error
}

type resolverService struct {
	config    *generic.Config
	service   Service
	resolvers []*Resolver
}

// NewResolverService returns a new instance of the view-sdk endpoint resolverService
func NewResolverService(config *generic.Config, service Service) (*resolverService, error) {
	er := &resolverService{
		config:  config,
		service: service,
	}
	if err := service.AddPKIResolver(NewPKIResolver()); err != nil {
		return nil, errors.Wrapf(err, "failed adding fabric pki resolver")
	}
	return er, nil
}

// LoadResolvers loads the resolvers specificed in the configuration file, if any
func (r *resolverService) LoadResolvers() error {
	// Load Resolver
	logger.Infof("loading resolvers")
	cfgResolvers, err := r.config.Resolvers()
	if err != nil {
		logger.Errorf("failed loading resolves [%s]", err)
		return err
	}
	logger.Infof("loaded resolves successfully, number of entries found %d", len(cfgResolvers))

	var resolvers []*Resolver
	for _, cfgResolver := range cfgResolvers {
		resolver := &Resolver{Resolver: cfgResolver}
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
		logger.Debugf("Resolver [%s,%s][%s] %s",
			resolver.Name, resolver.Domain, resolver.Addresses,
			view.Identity(resolver.Id).UniqueID(),
		)

		// Add Resolver
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
		resolvers = append(resolvers, resolver)
	}
	r.resolvers = resolvers
	return nil
}

// GetIdentity returns the identity associated to the passed label.
// The label is matched against the name
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
