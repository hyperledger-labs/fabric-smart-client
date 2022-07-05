/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"reflect"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/pkg/errors"
)

var (
	orionNetworkServiceType = reflect.TypeOf((*NetworkServiceProvider)(nil))
	logger                  = flogging.MustGetLogger("orion-sdk")
)

// NetworkService models a Orion Network
type NetworkService struct {
	SP        view2.ServiceProvider
	ons       driver.OrionNetworkService
	name      string
	committer *Committer
}

func NewNetworkService(SP view2.ServiceProvider, ons driver.OrionNetworkService, name string) *NetworkService {
	return &NetworkService{SP: SP, ons: ons, name: name, committer: NewCommitter(ons.Committer())}
}

// Name of this network
func (n *NetworkService) Name() string {
	return n.name
}

func (n *NetworkService) IdentityManager() *IdentityManager {
	return &IdentityManager{n.ons.IdentityManager()}
}

func (n *NetworkService) SessionManager() *SessionManager {
	return &SessionManager{n.ons.SessionManager()}
}

func (n *NetworkService) MetadataService() *MetadataService {
	return &MetadataService{ms: n.ons.MetadataService()}
}

// TransactionManager returns the transaction manager of this network
func (n *NetworkService) TransactionManager() *TransactionManager {
	return &TransactionManager{ons: n}
}

func (n *NetworkService) EnvelopeService() *EnvelopeService {
	return &EnvelopeService{es: n.ons.EnvelopeService()}
}

func (n *NetworkService) Vault() *Vault {
	return &Vault{ons: n.ons, v: n.ons.Vault()}
}

// Committer returns the committer service
func (n *NetworkService) Committer() *Committer {
	return n.committer
}

// ProcessorManager returns the processor manager of this network
func (n *NetworkService) ProcessorManager() *ProcessorManager {
	return &ProcessorManager{pm: n.ons.ProcessorManager()}
}

func (n *NetworkService) Finality() *Finality {
	return &Finality{finality: n.ons.Finality()}
}

type NetworkServiceProvider struct {
	sp              view2.ServiceProvider
	mutex           sync.RWMutex
	networkServices map[string]*NetworkService
}

func NewNetworkServiceProvider(sp view2.ServiceProvider) *NetworkServiceProvider {
	return &NetworkServiceProvider{sp: sp, networkServices: make(map[string]*NetworkService)}
}

func (nsp *NetworkServiceProvider) NetworkService(id string) (*NetworkService, error) {
	nsp.mutex.RLock()
	ns, ok := nsp.networkServices[id]
	nsp.mutex.RUnlock()
	if ok {
		return ns, nil
	}

	nsp.mutex.Lock()
	defer nsp.mutex.Unlock()
	ns, ok = nsp.networkServices[id]
	if ok {
		return ns, nil
	}

	provider := core.GetOrionNetworkServiceProvider(nsp.sp)
	if provider == nil {
		return nil, errors.New("no orion Network Service Provider found")
	}
	internalOns, err := provider.OrionNetworkService(id)
	if err != nil {
		return nil, errors.WithMessagef(err, "Failed to get orion Network Service for id [%s]", id)
	}
	ns, ok = nsp.networkServices[internalOns.Name()]
	if ok {
		return ns, nil
	}
	ns = NewNetworkService(nsp.sp, internalOns, internalOns.Name())
	nsp.networkServices[id] = ns
	nsp.networkServices[internalOns.Name()] = ns

	return ns, nil
}

func GetNetworkServiceProvider(sp view2.ServiceProvider) *NetworkServiceProvider {
	s, err := sp.GetService(orionNetworkServiceType)
	if err != nil {
		logger.Warnf("failed getting orion network service provider: %s", err)
		return nil
	}
	return s.(*NetworkServiceProvider)
}

func GetOrionNetworkNames(sp view2.ServiceProvider) []string {
	onsp := core.GetOrionNetworkServiceProvider(sp)
	if onsp == nil {
		return nil
	}
	return onsp.Names()
}

// GetOrionNetworkService returns the Orion Network Service for the passed id, nil if not found
func GetOrionNetworkService(sp view2.ServiceProvider, id string) *NetworkService {
	provider := GetNetworkServiceProvider(sp)
	if provider == nil {
		return nil
	}
	ons, err := provider.NetworkService(id)
	if err != nil {
		logger.Warnf("Failed to get orion Network Service for id [%s]: [%s]", id, err)
		return nil
	}
	return ons
}

// GetDefaultONS returns the default Orion Network Service
func GetDefaultONS(sp view2.ServiceProvider) *NetworkService {
	return GetOrionNetworkService(sp, "")
}
