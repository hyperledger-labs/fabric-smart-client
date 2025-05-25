/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package routing

import (
	"context"
	"math/rand"
	"sync/atomic"

	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
)

type SelectionStrategy[T any] func([]T) T

func AlwaysFirst[T any]() SelectionStrategy[T] {
	return func(sets []T) T {
		return sets[0]
	}
}
func AlwaysLast[T any]() SelectionStrategy[T] {
	return func(sets []T) T {
		return sets[len(sets)-1]
	}
}
func RoundRobin[T any]() SelectionStrategy[T] {
	it := uint64(0)
	return func(sets []T) T {
		return sets[atomic.AddUint64(&it, 1)%uint64(len(sets))]
	}
}

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

func (d *serviceDiscovery) LookupAll(ctx context.Context, id host2.PeerID) ([]host2.PeerIPAddress, bool) {
	return d.router.Lookup(ctx, id)
}
func (d *serviceDiscovery) Lookup(ctx context.Context, id host2.PeerID) host2.PeerIPAddress {
	if endpoints, ok := d.router.Lookup(ctx, id); ok && len(endpoints) > 0 {
		return d.strategy(endpoints)
	}
	return ""
}

func NewServiceDiscovery(router IDRouter, strategy EndpointSelector) *serviceDiscovery {
	return &serviceDiscovery{
		router:   router,
		strategy: strategy,
	}
}
