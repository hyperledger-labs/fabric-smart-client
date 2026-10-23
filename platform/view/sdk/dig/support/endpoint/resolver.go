/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint

import (
	"context"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var logger = logging.MustGetLogger()

type Identity struct {
	Path string `yaml:"path"`
}

type entry struct {
	Name           string            `yaml:"name,omitempty"`
	Domain         string            `yaml:"domain,omitempty"`
	Identity       Identity          `yaml:"identity,omitempty"`
	Addresses      map[string]string `yaml:"addresses,omitempty"`
	Aliases        []string          `yaml:"aliases,omitempty"`
	ID             []byte
	IdentityGetter func() (view.Identity, []byte, error)
}

func (r *entry) GetIdentity() (view.Identity, error) {
	if r.IdentityGetter != nil {
		id, _, err := r.IdentityGetter()
		return id, err
	}

	return r.ID, nil
}

type ConfigService interface {
	GetString(key string) string
	IsSet(s string) bool
	UnmarshalKey(s string, i any) error
	TranslatePath(path string) string
}

type IdentityService interface {
	DefaultIdentity() view.Identity
}

type Backend interface {
	Bind(ctx context.Context, longTerm view.Identity, ephemeral ...view.Identity) error
	AddResolver(name, domain string, addresses map[string]string, aliases []string, id []byte) (view.Identity, error)
}

type ResolversLoader struct {
	config  ConfigService
	backend Backend
	is      IdentityService
}

// NewResolversLoader returns a new instance of ResolversLoader that loads resolver from the configuration.
func NewResolversLoader(config ConfigService, backend Backend, is IdentityService) (*ResolversLoader, error) {
	er := &ResolversLoader{
		config:  config,
		backend: backend,
		is:      is,
	}
	return er, nil
}

// LoadResolvers registers the default resolver for this node, built from fsc.id,
// fsc.grpc.address and fsc.p2p.listenAddress, followed by each entry of
// fsc.endpoint.resolvers. fsc.id must be set, and an entry must have a name and an
// identity.path; LoadResolvers returns an error naming the key or entry otherwise, or if
// the backend rejects a resolver.
func (r *ResolversLoader) LoadResolvers() error {
	// add default
	p2pAddress := r.config.GetString("fsc.p2p.listenAddress")
	address, err := convertAddress(r.config.GetString("fsc.p2p.listenAddress"))
	if err != nil {
		return errors.Wrapf(err, "failed to convert address [%s]", p2pAddress)
	}
	nodeID := r.config.GetString("fsc.id")
	if len(nodeID) == 0 {
		return errors.New("fsc.id is required")
	}
	_, err = r.backend.AddResolver(
		nodeID,
		"",
		map[string]string{
			string(endpoint.ViewPort): r.config.GetString("fsc.grpc.address"),
			string(endpoint.P2PPort):  address,
		},
		nil,
		r.is.DefaultIdentity(),
	)
	if err != nil {
		logger.Errorf("failed adding default resolver [%s]", err)
		return errors.Wrap(err, "failed adding default resolver")
	}

	// Load entry
	if r.config.IsSet("fsc.endpoint.resolvers") {
		logger.Debugf("loading resolvers")
		var resolvers []*entry
		err := r.config.UnmarshalKey("fsc.endpoint.resolvers", &resolvers)
		if err != nil {
			logger.Errorf("failed loading resolvers [%s]", err)
			return errors.Wrap(err, "failed loading resolvers")
		}
		logger.Debugf("loaded resolvers successfully, number of entries found %d", len(resolvers))

		for i, resolver := range resolvers {
			if len(resolver.Name) == 0 {
				return errors.Errorf("fsc.endpoint.resolvers[%d]: name is required", i)
			}
			if len(resolver.Identity.Path) == 0 {
				return errors.Errorf("fsc.endpoint.resolvers[%d] [%s]: identity.path is required", i, resolver.Name)
			}
			raw, err := id.LoadIdentity(r.config.TranslatePath(resolver.Identity.Path))
			if err != nil {
				return errors.WithMessagef(err, "fsc.endpoint.resolvers[%d] [%s]: failed loading identity.path [%s]", i, resolver.Name, resolver.Identity.Path)
			}
			resolver.ID = raw
			logger.Debugf("resolver [%s,%s][%s] %s",
				resolver.Name, resolver.Domain, resolver.Addresses,
				view.Identity(resolver.ID).UniqueID(),
			)

			// Add entry
			if _, err := r.backend.AddResolver(resolver.Name, resolver.Domain, resolver.Addresses, resolver.Aliases, resolver.ID); err != nil {
				return errors.WithMessagef(err, "fsc.endpoint.resolvers[%d]", i)
			}

			// Bind Aliases
			for _, alias := range resolver.Aliases {
				logger.Debugf("binding [%s] to [%s]", resolver.Name, alias)
				if err := r.backend.Bind(context.Background(), resolver.ID, []byte(alias)); err != nil {
					return errors.WithMessagef(err, "failed binding identity [%s] to alias [%s]", resolver.Name, alias)
				}
			}
		}
	}
	return nil
}

func convertAddress(addr string) (string, error) {
	result, err := comm.ConvertAddress(addr)
	if err != nil {
		return "", err
	}
	if after, ok := strings.CutPrefix(result, "0.0.0.0"); ok {
		// change the prefix to 127.0.0.1
		result = "127.0.0.1" + after
	}
	return result, nil
}
