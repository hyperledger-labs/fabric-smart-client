/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest

import host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"

type mapRouteProvider struct {
	routes *mapRouter
}

func newMapRouteProvider(routes *mapRouter) *mapRouteProvider {
	return &mapRouteProvider{routes: routes}
}

func (p *mapRouteProvider) NewBootstrapHost(listenAddress host2.PeerIPAddress, privateKeyPath, certPath string) (host2.P2PHost, error) {
	nodeID, _ := p.routes.reverseLookup(listenAddress)
	return NewHost(nodeID, listenAddress, p.routes, privateKeyPath, certPath, nil)
}

func (p *mapRouteProvider) NewHost(listenAddress host2.PeerIPAddress, privateKeyPath, certPath string, _ host2.PeerIPAddress) (host2.P2PHost, error) {
	return p.NewBootstrapHost(listenAddress, privateKeyPath, certPath)
}
