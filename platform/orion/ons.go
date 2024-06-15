/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"reflect"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/pkg/errors"
)

var (
	orionNetworkServiceType = reflect.TypeOf((*NetworkServiceProvider)(nil))
)

// NetworkService models an Orion network
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

func (n *NetworkService) Vault() Vault {
	return newVault(n.ons)
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
	onsProvider     driver.OrionNetworkServiceProvider
	mutex           sync.RWMutex
	networkServices map[string]*NetworkService
}

func NewNetworkServiceProvider(sp view2.ServiceProvider, onsProvider driver.OrionNetworkServiceProvider) *NetworkServiceProvider {
	return &NetworkServiceProvider{
		sp:              sp,
		onsProvider:     onsProvider,
		networkServices: make(map[string]*NetworkService),
	}
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

	internalOns, err := nsp.onsProvider.OrionNetworkService(id)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to get orion Network Service for id [%s]", id)
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

func GetNetworkServiceProvider(sp view2.ServiceProvider) (*NetworkServiceProvider, error) {
	s, err := sp.GetService(orionNetworkServiceType)
	if err != nil {
		return nil, err
	}
	return s.(*NetworkServiceProvider), nil
}

// GetOrionNetworkService returns the Orion Network Service for the passed id, nil if not found
func GetOrionNetworkService(sp view2.ServiceProvider, id string) (*NetworkService, error) {
	provider, err := GetNetworkServiceProvider(sp)
	if err != nil {
		return nil, err
	}
	ons, err := provider.NetworkService(id)
	if err != nil {
		return nil, err
	}
	return ons, nil
}

// GetDefaultONS returns the default Orion Network Service
func GetDefaultONS(sp view2.ServiceProvider) (*NetworkService, error) {
	return GetOrionNetworkService(sp, "")
}
