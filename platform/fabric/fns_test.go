/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser/mock"
)

func TestNetworkService(t *testing.T) {
	t.Parallel()

	mfns := &mock.FabricNetworkService{}
	ns := NewNetworkService(nil, mfns, "mynetwork")

	require.Equal(t, "mynetwork", ns.Name())

	// Channel
	mch := &mock.Channel{}
	mch.NameReturns("mychannel")
	mfns.ChannelReturns(mch, nil)

	ch, err := ns.Channel("mychannel")
	require.NoError(t, err)
	require.NotNil(t, ch)
	require.Equal(t, "mychannel", ch.Name())

	// again from cache: same instance, and n.fns.Channel consulted only once
	ch2, err := ns.Channel("mychannel")
	require.NoError(t, err)
	require.Same(t, ch, ch2)
	require.Equal(t, 1, mfns.ChannelCallCount())

	// IdentityProvider
	require.NotNil(t, ns.IdentityProvider())
	// LocalMembership
	require.NotNil(t, ns.LocalMembership())
	// Ordering
	require.NotNil(t, ns.Ordering())
	// ProcessorManager
	require.NotNil(t, ns.ProcessorManager())
	// TransactionManager
	require.NotNil(t, ns.TransactionManager())
	// SignerService
	require.NotNil(t, ns.SignerService())
	// ConfigService
	require.NotNil(t, ns.ConfigService())
}

func TestNetworkServiceProvider(t *testing.T) {
	t.Parallel()

	mfnsProv := &mock.FabricNetworkServiceProvider{}
	nsp := NewNetworkServiceProvider(mfnsProv, nil)

	mfns := &mock.FabricNetworkService{}
	mfns.NameReturns("defaultnet")
	mfnsProv.FabricNetworkServiceReturns(mfns, nil)

	ns, err := nsp.FabricNetworkService("defaultnet")
	require.NoError(t, err)
	require.NotNil(t, ns)
	require.Equal(t, "defaultnet", ns.Name())

	// cache: same instance, and the underlying provider consulted only once
	ns2, err := nsp.FabricNetworkService("defaultnet")
	require.NoError(t, err)
	require.Same(t, ns, ns2)
	require.Equal(t, 1, mfnsProv.FabricNetworkServiceCallCount())

	// test Get functions: the service provider fails to resolve the provider
	msp := &dummyServiceProvider{}

	names, err := GetFabricNetworkNames(msp)
	require.ErrorContains(t, err, "failed getting fabric network service provider")
	require.Nil(t, names)

	_, err = GetDefaultFNS(msp)
	require.ErrorContains(t, err, "failed getting fabric network service provider")

	_, _, err = GetDefaultChannel(msp)
	require.ErrorContains(t, err, "failed getting fabric network service provider")

	_, _, err = GetChannel(msp, "net", "chan")
	require.ErrorContains(t, err, "failed getting fabric network service provider")

	// now the provider resolves, but the underlying fns lookup fails
	provider := NewNetworkServiceProvider(mfnsProv, nil)
	msp.getServiceReturn = provider

	mfnsProv.FabricNetworkServiceReturns(nil, errors.New("no fns"))

	_, err = GetDefaultFNS(msp)
	require.ErrorContains(t, err, "no fns")

	_, _, err = GetDefaultChannel(msp)
	require.ErrorContains(t, err, "no fns")

	_, _, err = GetChannel(msp, "net", "chan")
	require.ErrorContains(t, err, "no fns")
}

func TestFabricNetworkFreeFunctions(t *testing.T) {
	t.Parallel()

	mfnsProv := &mock.FabricNetworkServiceProvider{}
	mfns := &mock.FabricNetworkService{}
	mfns.NameReturns("defaultnet")
	mfnsProv.FabricNetworkServiceReturns(mfns, nil)

	mch := &mock.Channel{}
	mch.NameReturns("mychannel")
	mfns.ChannelReturns(mch, nil)

	// GetDefaultFNS/GetDefaultChannel/GetChannel resolve the provider through
	// GetNetworkServiceProvider, which type-asserts *NetworkServiceProvider, so the
	// service provider must hand back a *NetworkServiceProvider.
	provider := NewNetworkServiceProvider(mfnsProv, nil)
	sp := &dummyServiceProvider{getServiceReturn: provider}

	defFNS, err := GetDefaultFNS(sp)
	require.NoError(t, err)
	require.Equal(t, "defaultnet", defFNS.Name())

	net, ch, err := GetDefaultChannel(sp)
	require.NoError(t, err)
	require.Equal(t, "defaultnet", net.Name())
	require.Equal(t, "mychannel", ch.Name())

	net, ch, err = GetChannel(sp, "somenet", "mychannel")
	require.NoError(t, err)
	require.Equal(t, "defaultnet", net.Name())
	require.Equal(t, "mychannel", ch.Name())

	// GetFabricNetworkNames resolves through core.GetFabricNetworkServiceProvider, which
	// type-asserts driver.FabricNetworkServiceProvider, so hand back the mock provider
	// directly (it implements that interface).
	mfnsProv.NamesReturns([]string{"net1", "net2"})
	spNames := &dummyServiceProvider{getServiceReturn: mfnsProv}
	names, err := GetFabricNetworkNames(spNames)
	require.NoError(t, err)
	require.Equal(t, []string{"net1", "net2"}, names)
}

type dummyServiceProvider struct {
	getServiceReturn any
}

func (d *dummyServiceProvider) GetService(v any) (any, error) {
	if d.getServiceReturn != nil {
		return d.getServiceReturn, nil
	}
	return nil, errors.New("err")
}
