/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint

import (
	"net"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"golang.org/x/exp/constraints"
)

type AddressSet = map[driver.PortName]string

func contains(addressSets []AddressSet, address string) bool {
	for _, addressSet := range addressSets {
		for _, a := range addressSet {
			if a == address {
				return true
			}
		}
	}
	return false
}

func newAddressSet(addresses map[string]string) AddressSet {
	addressSet := make(AddressSet, len(addresses))
	for k, v := range addresses {
		addressSet[portNameMap[strings.ToLower(k)]] = lookupIPv4(v)
	}
	return addressSet
}

func lookupIPv4(endpoint string) string {
	s := strings.Split(endpoint, ":")
	if len(s) < 2 {
		return endpoint
	}
	var addrS string
	addr, err := net.LookupIP(s[0])
	if err != nil {
		addrS = s[0]
	} else {
		addrS = addr[0].String()
	}
	port := s[1]
	return net.JoinHostPort(addrS, port)
}

var portNameMap = map[string]driver.PortName{
	strings.ToLower(string(driver.ListenPort)): driver.ListenPort,
	strings.ToLower(string(driver.ViewPort)):   driver.ViewPort,
	strings.ToLower(string(driver.P2PPort)):    driver.P2PPort,
}

type Set[T constraints.Ordered] map[T]struct{}

func newSet[T constraints.Ordered](aliases []T) Set[T] {
	a := make(Set[T], len(aliases))
	a.Add(aliases...)
	return a
}
func (a *Set[T]) Contains(alias T) bool {
	_, ok := (*a)[alias]
	return ok
}
func (a *Set[T]) Add(aliases ...T) {
	for _, alias := range aliases {
		(*a)[alias] = struct{}{}
	}
}
