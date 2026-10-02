/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package routing

import (
	"math/rand"
	"sync/atomic"

	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
)

// SelectionStrategy picks one element of a non-empty slice. The strategies in
// this package panic on an empty slice; callers must check the length first.
type SelectionStrategy[T any] func([]T) T

// AlwaysFirst returns a strategy that always picks the first element.
func AlwaysFirst[T any]() SelectionStrategy[T] {
	return func(sets []T) T {
		return sets[0]
	}
}

// RoundRobin returns a strategy that cycles through the elements. It is safe
// for concurrent use; each call to RoundRobin returns a strategy with its own
// counter.
func RoundRobin[T any]() SelectionStrategy[T] {
	it := uint64(0)
	return func(sets []T) T {
		return sets[atomic.AddUint64(&it, 1)%uint64(len(sets))]
	}
}

// Random returns a strategy that picks an element at random.
func Random[T any]() SelectionStrategy[T] {
	return func(sets []T) T {
		return sets[rand.Int()%len(sets)]
	}
}

type EndpointSelector = SelectionStrategy[host2.PeerIPAddress]

type serviceDiscovery struct {
	router   IDRouter
	strategy EndpointSelector
}

func (d *serviceDiscovery) LookupAll(id host2.PeerID) ([]host2.PeerIPAddress, bool) {
	return d.router.Lookup(id)
}

func (d *serviceDiscovery) Lookup(id host2.PeerID) host2.PeerIPAddress {
	if endpoints, ok := d.router.Lookup(id); ok && len(endpoints) > 0 {
		return d.strategy(endpoints)
	}
	return ""
}

// NewServiceDiscovery returns a ServiceDiscovery that looks up addresses with
// router and picks one with strategy. Lookup returns the empty address when
// the router finds no address, so strategy never sees an empty slice.
func NewServiceDiscovery(router IDRouter, strategy EndpointSelector) *serviceDiscovery {
	return &serviceDiscovery{
		router:   router,
		strategy: strategy,
	}
}
