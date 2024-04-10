/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint

import (
	"os"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/x509"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
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
	AddPublicKeyExtractor(extractor view2.PublicKeyExtractor) error
}

type Config interface {
	Resolvers() ([]config.Resolver, error)
	TranslatePath(path string) string
}

type ResolverService struct {
	config    Config
	service   Service
	resolvers []*Resolver
}

// NewResolverService returns a new instance of the view-sdk endpoint resolverService
func NewResolverService(config Config, service Service) (*ResolverService, error) {
	er := &ResolverService{
		config:  config,
		service: service,
	}
	if err := service.AddPublicKeyExtractor(PublicKeyExtractor{}); err != nil {
		return nil, errors.Wrapf(err, "failed adding fabric pki resolver")
	}
	return er, nil
}

// LoadResolvers loads the resolvers specified in the configuration file, if any
func (r *ResolverService) LoadResolvers() error {
	// Load Resolver
	logger.Infof("loading resolvers")
	cfgResolvers, err := r.config.Resolvers()
	if err != nil {
		logger.Errorf("failed loading resolvers [%s]", err)
		return err
	}
	logger.Infof("loaded resolvers successfully, number of entries found %d", len(cfgResolvers))

	var resolvers []*Resolver
	for _, cfgResolver := range cfgResolvers {
		resolver := &Resolver{Resolver: cfgResolver}

		path := r.config.TranslatePath(resolver.Identity.Path)
		var raw []byte
		// Load identity, is resolver.Identity.Path a directory or a file?
		fileInfo, err := os.Stat(path)
		if err != nil {
			return errors.Wrapf(err, "failed to stat [%s]", path)
		}
		if fileInfo.IsDir() {
			// load the msp provider
			raw, err = x509.SerializeFromMSP(resolver.Identity.MSPID, path)
			if err != nil {
				return errors.Wrapf(err, "failed to load msp at [%s:%s]", resolver.Identity.MSPID, path)
			}
		} else {
			// file is not a directory
			logger.Warnf("resolver [%s:%s] points directly to a certificate, cannot sanitize, use an msp folder instead", resolver.Identity.MSPID, path)
			raw, err = x509.Serialize(resolver.Identity.MSPID, path)
			if err != nil {
				return errors.Wrapf(err, "failed to load  x509 certicate [%s:%s]", resolver.Identity.MSPID, path)
			}
		}

		resolver.Id = raw
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("Resolver [%s,%s][%s] %s",
				resolver.Name, resolver.Domain, resolver.Addresses,
				view.Identity(resolver.Id).UniqueID(),
			)
		}

		// Add Resolver
		rootID, err := r.service.AddResolver(resolver.Name, resolver.Domain, resolver.Addresses, resolver.Aliases, resolver.Id)
		if err != nil {
			return errors.Wrapf(err, "failed adding resolver")
		}
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("added resolver [root-id:%s]", rootID.String())
		}
		resolver.RootID = rootID

		// Bind Aliases
		for _, alias := range resolver.Aliases {
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("binding [%s] to [%s]", resolver.Name, alias)
			}
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
func (r *ResolverService) GetIdentity(label string) view.Identity {
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
