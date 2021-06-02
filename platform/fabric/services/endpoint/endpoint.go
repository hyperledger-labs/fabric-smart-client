/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package endpoint

import (
	"bytes"
	"runtime/debug"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/x509"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/api"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

const (
	idemixMSP = "idemix"
	bccspMSP  = "bccsp"
)

var logger = flogging.MustGetLogger("fabric-sdk.endpoint")

type MspConf struct {
	ID      string `yaml:"id"`
	MSPType string `yaml:"mspType"`
	MSPID   string `yaml:"mspID"`
	Path    string `yaml:"path"`
}

type resolver struct {
	Name           string            `yaml:"name,omitempty"`
	Domain         string            `yaml:"domain,omitempty"`
	Identity       MspConf           `yaml:"identity,omitempty"`
	Addresses      map[string]string `yaml:"addresses,omitempty"`
	Aliases        []string          `yaml:"aliases,omitempty"`
	Id             []byte
	IdentityGetter func() (view.Identity, []byte, error)
}

func (r *resolver) GetIdentity() (view.Identity, error) {
	if r.IdentityGetter != nil {
		id, _, err := r.IdentityGetter()
		return id, err
	}

	return r.Id, nil
}

// NetworkMember is a peer's representation
type NetworkMember struct {
	Endpoint         string
	Metadata         []byte
	PKIid            []byte
	InternalEndpoint string
}

type PKIResolver interface {
	GetPKIidOfCert(peerIdentity view.Identity) []byte
}

type Discovery interface {
	Peers() []NetworkMember
}

type endpointEntry struct {
	Endpoints map[api.PortName]string
	Ephemeral view.Identity
	Identity  view.Identity
}

type service struct {
	sp          view2.ServiceProvider
	resolvers   []*resolver
	Discovery   Discovery
	pkiResolver PKIResolver
}

// NewService returns a new instance of the view-sdk endpoint service
func NewService(sp view2.ServiceProvider, discovery Discovery, pkiResolver PKIResolver) (*service, error) {
	er := &service{
		sp:          sp,
		Discovery:   discovery,
		pkiResolver: pkiResolver,
	}
	if err := er.init(); err != nil {
		return nil, err
	}
	return er, nil
}

func (r *service) Endpoint(party view.Identity) (map[api.PortName]string, error) {
	for _, resolver := range r.resolvers {
		if bytes.Equal(resolver.Id, party) {
			return convert(resolver.Addresses), nil
		}
	}
	// lookup for the binding
	endpointEntry, err := r.getBinding(party.UniqueID())
	if err != nil {
		return nil, errors.Wrapf(err, "endpoint not found for identity %s", party.UniqueID())
	}
	return endpointEntry.Endpoints, nil
}

func (r *service) Resolve(party view.Identity) (view.Identity, map[api.PortName]string, []byte, error) {
	pkiR := r.pkiResolver

	e, err := r.endpointInternal(party)
	if err != nil {
		logger.Debugf("resolving via binding for %s", view.Identity(pkiR.GetPKIidOfCert(party)))
		ee, err := r.getBinding(party.UniqueID())
		if err != nil {
			return nil, nil, nil, errors.Wrapf(err, "endpoint not found for identity [%s,%s]", string(party), party.UniqueID())
		}

		if len(ee.Endpoints) != 0 {
			e = ee.Endpoints
		} else {
			e, err = r.Endpoint(ee.Identity)
			if err != nil {
				return nil, nil, nil, errors.Errorf("endpoint not found for identity [%s]", party.UniqueID())
			}
		}

		logger.Debugf("resolved to [%s,%s,%s]", view.Identity(pkiR.GetPKIidOfCert(party)), e, pkiR.GetPKIidOfCert(ee.Identity))
		return ee.Identity, e, pkiR.GetPKIidOfCert(ee.Identity), nil
	}

	return party, e, pkiR.GetPKIidOfCert(party.Bytes()), nil
}

func (r *service) Bind(longTerm view.Identity, ephemeral view.Identity) error {
	e, err := r.Endpoint(longTerm)
	if err != nil {
		return errors.Errorf("long term identity not found for identity [%s]", longTerm.UniqueID())
	}
	if err := r.putBinding(ephemeral.UniqueID(), &endpointEntry{Endpoints: e, Identity: longTerm, Ephemeral: ephemeral}); err != nil {
		return errors.WithMessagef(err, "failed storing binding of [%s]  to [%s]", ephemeral.UniqueID(), longTerm.UniqueID())
	}

	return nil
}

func (r *service) GetIdentity(endpoint string, pkid []byte) (view.Identity, error) {
	// search in the resolver list
	pkiResolver := r.pkiResolver
	for _, resolver := range r.resolvers {
		resolverPKID := pkiResolver.GetPKIidOfCert(resolver.Id)

		found := false
		for _, addr := range resolver.Addresses {
			if endpoint == addr {
				found = true
				break
			}
		}

		if endpoint == resolver.Name ||
			found ||
			endpoint == resolver.Name+"."+resolver.Domain ||
			bytes.Equal(pkid, resolver.Id) ||
			bytes.Equal(pkid, resolverPKID) {

			id, err := resolver.GetIdentity()
			if err != nil {
				return nil, err
			}
			logger.Infof("resolving [%s,%s] to %s", endpoint, view.Identity(pkid), id)
			return id, nil
		}
	}

	// ask the msp service
	id, err := fabric.GetDefaultNetwork(r.sp).LocalMembership().GetIdentityByID(endpoint)
	if err != nil {
		return nil, errors.Errorf("identity not found at [%s,%s] %s [%s]", endpoint, view.Identity(pkid), debug.Stack(), err)
	}
	return id, nil

	// ask discovery
	//for _, member := range r.Discovery.Peers() {
	//	if endpoint == member.Endpoint ||
	//		bytes.Equal(pkid, member.PKIid) {
	//		return []byte(member.PKIid), nil
	//	}
	//}
}

func (r *service) AddLongTermIdentity(identity view.Identity) error {
	return r.putBinding(identity.String(), &endpointEntry{
		Identity: identity,
	})
}

func (r *service) init() error {
	// Load resolver
	configProvider := view2.GetConfigService(r.sp)

	if configProvider.IsSet("fabric.endpoint.resolves") {
		logger.Infof("loading resolvers")
		err := configProvider.UnmarshalKey("fabric.endpoint.resolves", &r.resolvers)
		if err != nil {
			logger.Errorf("failed loading resolves [%s]", err)
			return err
		}
		logger.Infof("loaded resolves successfully, number of entries found %d", len(r.resolvers))

		for _, resolver := range r.resolvers {
			// Load identity
			var raw []byte
			switch resolver.Identity.MSPType {
			case bccspMSP:
				raw, err = x509.Serialize(resolver.Identity.MSPID, configProvider.TranslatePath(resolver.Identity.Path))
				if err != nil {
					return errors.Wrapf(err, "failed serializing x509 identity")
				}
			default:
				return errors.Errorf("expected bccsp type, got %s", resolver.Identity.MSPType)
			}
			resolver.Id = raw
			logger.Infof("resolver [%s,%s][%s] %s",
				resolver.Name, resolver.Domain, resolver.Addresses,
				view.Identity(resolver.Id).UniqueID(),
			)

			for _, alias := range resolver.Aliases {
				logger.Debugf("binging [%s] to [%s]", resolver.Name, alias)
				if err := r.Bind(resolver.Id, []byte(alias)); err != nil {
					return errors.WithMessagef(err, "failed binding identity [%s] to alias [%s]", resolver.Name, alias)
				}
			}
		}
	}

	return nil
}

func (r *service) endpointInternal(party view.Identity) (map[api.PortName]string, error) {
	for _, resolver := range r.resolvers {
		if bytes.Equal(resolver.Id, party) {
			return convert(resolver.Addresses), nil
		}
	}
	return nil, errors.Errorf("endpoint not found for identity %s", party.UniqueID())
}

func (r *service) putBinding(key string, entry *endpointEntry) error {
	k := kvs.CreateCompositeKeyOrPanic(
		"platform.fabric.endpoint.binding",
		[]string{key},
	)
	kvss := kvs.GetService(r.sp)
	if err := kvss.Put(k, entry); err != nil {
		return err
	}
	return nil
}

func (r *service) getBinding(key string) (*endpointEntry, error) {
	k := kvs.CreateCompositeKeyOrPanic(
		"platform.fabric.endpoint.binding",
		[]string{key},
	)
	kvss := kvs.GetService(r.sp)
	if !kvss.Exists(k) {
		return nil, errors.Errorf("binding not found for [%s]", key)
	}
	entry := &endpointEntry{}
	if err := kvss.Get(k, entry); err != nil {
		return nil, err
	}
	return entry, nil
}

func convert(o map[string]string) map[api.PortName]string {
	r := map[api.PortName]string{}
	for k, v := range o {
		r[api.PortName(k)] = v
	}
	return r
}
