/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config_test

import (
	"errors"
	"testing"
	"time"

	cfg "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	sdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/stretchr/testify/require"
)

func TestNewService_missingConfig(t *testing.T) {
	m := &mock.Configuration{}
	m.IsSetReturns(false)
	// when defaultConfig=false and fabric.<name> is not set, error expected
	_, err := cfg.NewService(m, "mynet", false)
	require.Error(t, err)
}

func TestNewService_defaultsAndOrderers(t *testing.T) {
	m := &mock.Configuration{}
	// simulate fabirc.mynet present
	m.IsSetReturnsOnCall(0, true)   // called for fabric.mynet check
	m.GetStringReturnsOnCall(0, "") // fabric.mynetdriver -> default
	// enable TLS so TLSRootCertFile gets translated
	m.GetBoolReturns(true)

	// orderers: return a slice with one connection config
	orderers := []*grpc.ConnectionConfig{{Address: "o:7050", TLSRootCertFile: "o.pem"}}
	m.UnmarshalKeyStub = func(key string, rawVal interface{}) error {
		// support both key formats used across tests
		switch key {
		case "fabric.mynet.orderers", "fabric.mynetorderers":
			p, ok := rawVal.(*[]*grpc.ConnectionConfig)
			if !ok {
				return nil
			}
			*p = orderers
			return nil
		case "fabric.mynet.peers", "fabric.mynetpeers":
			p, ok := rawVal.(*[]*grpc.ConnectionConfig)
			if !ok {
				return nil
			}
			*p = []*grpc.ConnectionConfig{{Address: "p:7051", Usage: "query"}}
			return nil
		case "fabric.mynet.channels", "fabric.mynetchannels":
			p, ok := rawVal.(*[]*cfg.Channel)
			if !ok {
				return nil
			}
			*p = []*cfg.Channel{{Name: "ch1", Default: true}}
			return nil
		}
		return nil
	}
	m.TranslatePathReturns("TRANSLATED:o.pem")

	svc, err := cfg.NewService(m, "mynet", false)
	require.NoError(t, err)
	require.Equal(t, "mynet", svc.NetworkName())
	require.Equal(t, cfg.GenericDriver, svc.DriverName())
	require.Equal(t, 1, len(svc.Orderers()))
	require.Equal(t, "TRANSLATED:o.pem", svc.Orderers()[0].TLSRootCertFile)
	require.Equal(t, "ch1", svc.DefaultChannel())
}

func TestClientKeepAliveConfig_UnmarshalError(t *testing.T) {
	m := &mock.Configuration{}
	m.IsSetReturns(true) // keepalive.interval is set
	m.UnmarshalKeyReturnsOnCall(0, errors.New("boom"))

	svc := &cfg.Service{Configuration: m}
	// should return nil on unmarshal error
	k := svc.ClientKeepAliveConfig()
	require.Nil(t, k)
}

func TestVaultAndMSPSettings(t *testing.T) {
	m := &mock.Configuration{}
	// fabric prefix empty
	m.GetStringReturnsOnCall(0, "persistenceName") // vault.persistence
	m.GetStringReturnsOnCall(1, "50")              // vault.txidstore.cache.size (string that parses)
	m.GetStringReturnsOnCall(2, "defaultMSP")      // defaultMSP
	m.UnmarshalKeyStub = func(key string, rawVal interface{}) error {
		if key == "fabric.msps" {
			p, ok := rawVal.(*[]cfg.MSP)
			if !ok {
				return nil
			}
			*p = []cfg.MSP{{ID: "msp1"}}
			return nil
		}
		return nil
	}
	svc := &cfg.Service{Configuration: m}
	require.Equal(t, sdriver.PersistenceName("persistenceName"), svc.VaultPersistenceName())
	require.Equal(t, 50, svc.VaultTXStoreCacheSize())
	require.Equal(t, "defaultMSP", svc.DefaultMSP())
	msps, err := svc.MSPs()
	require.NoError(t, err)
	require.Len(t, msps, 1)
}

func TestChannelHelpers(t *testing.T) {
	ch := &cfg.Channel{}
	// default values
	require.Equal(t, time.Duration(5*time.Minute), ch.DiscoveryDefaultTTLS())
	require.Equal(t, 1, ch.CommitParallelism())
	require.Equal(t, 100*time.Millisecond, ch.CommitterPollingTimeout())
	require.Equal(t, 10*time.Second, ch.DeliverySleepAfterFailure())
	require.Equal(t, 20*time.Second, ch.FinalityWaitTimeout())
	require.Equal(t, 1, ch.FinalityEventQueueWorkers())
	require.Equal(t, 300*time.Second, ch.CommitterWaitForEventTimeout())
	require.Equal(t, 1, ch.DeliveryBufferSize())
	require.Equal(t, 20*time.Second, ch.DiscoveryTimeout())
	require.Equal(t, 3, ch.CommitterFinalityNumRetries())
	require.Equal(t, time.Duration(100*time.Millisecond), ch.CommitterFinalityUnknownTXTimeout())
	require.Equal(t, time.Minute, ch.FinalityForPartiesWaitTimeout())

	// ChaincodeConfigs should convert to driver.ChaincodeConfig
	cc := &cfg.Chaincode{Name: "cc1", Private: true}
	c := &cfg.Channel{Chaincodes: []*cfg.Chaincode{cc}}
	arr := c.ChaincodeConfigs()
	require.Len(t, arr, 1)
	require.Equal(t, "cc1", arr[0].ID())
}

func TestCreatePeerMapAndPickPeer(t *testing.T) {
	m := &mock.Configuration{}
	m.IsSetReturnsOnCall(0, true) // fabric.network present for NewService
	m.GetStringReturnsOnCall(0, "")
	m.GetBoolReturns(true) // TLS enabled
	m.UnmarshalKeyStub = func(key string, rawVal interface{}) error {
		switch key {
		case "fabric.test.peers", "fabric.testpeers":
			p, ok := rawVal.(*[]*grpc.ConnectionConfig)
			if !ok {
				return nil
			}
			*p = []*grpc.ConnectionConfig{
				{Address: "p1", Usage: "query", TLSRootCertFile: "r1.pem"},
				{Address: "p2", Usage: "delivery"},
			}
			return nil
		case "fabric.test.orderers", "fabric.testorderers":
			p, ok := rawVal.(*[]*grpc.ConnectionConfig)
			if !ok {
				return nil
			}
			*p = []*grpc.ConnectionConfig{{Address: "o1"}}
			return nil
		case "fabric.test.channels", "fabric.testchannels":
			p, ok := rawVal.(*[]*cfg.Channel)
			if !ok {
				return nil
			}
			*p = []*cfg.Channel{{Name: "ch1"}}
			return nil
		}
		return nil
	}
	m.TranslatePathReturns("TR")

	svc, err := cfg.NewService(m, "test", false)
	require.NoError(t, err)

	// pick a peer for query — ensure non-nil
	p := svc.PickPeer(driver.PeerForQuery)
	require.NotNil(t, p)
}

func TestPickOrderer_nilAndSetConfigOrderers(t *testing.T) {
	m := &mock.Configuration{}
	svc := &cfg.Service{Configuration: m}
	// nil case
	require.Nil(t, svc.PickOrderer())

	// set orderers and ensure pick returns one (via SetConfigOrderers)
	newOrderers := []*grpc.ConnectionConfig{{Address: "o1"}, {Address: "o2"}}
	require.NoError(t, svc.SetConfigOrderers(newOrderers))
	picked := svc.PickOrderer()
	require.NotNil(t, picked)
}

func TestServiceGetters(t *testing.T) {
	m := &mock.Configuration{}
	// Initialize with some basic setup to avoid NewService errors
	m.IsSetReturns(true)
	m.GetStringReturns("")
	svc, err := cfg.NewService(m, "mynet", true)
	require.NoError(t, err)

	// Reset mock for subsequent calls to have more control
	m.IsSetReturns(false)
	m.GetBoolReturns(false)
	m.GetStringReturns("")

	// Ordering TLS settings
	enabled, ok := svc.OrderingTLSEnabled()
	require.True(t, enabled) // default when not set
	require.False(t, ok)

	m.IsSetReturns(true)
	m.GetBoolReturns(true)
	enabled, ok = svc.OrderingTLSEnabled()
	require.True(t, enabled)
	require.True(t, ok)

	m.IsSetReturns(false)
	required, ok := svc.OrderingTLSClientAuthRequired()
	require.False(t, required)
	require.False(t, ok)

	m.IsSetReturns(true)
	m.GetBoolReturns(true)
	required, ok = svc.OrderingTLSClientAuthRequired()
	require.True(t, required)
	require.True(t, ok)

	// TLS settings
	m.GetBoolReturns(true)
	require.True(t, svc.TLSClientAuthRequired())

	m.GetStringReturns("server-host")
	require.Equal(t, "server-host", svc.TLSServerHostOverride())

	// Keepalive
	m.IsSetReturns(false)
	require.Equal(t, 10*time.Second, svc.ClientConnTimeout())

	m.IsSetReturns(true)
	m.GetDurationReturns(5 * time.Second)
	require.Equal(t, 5*time.Second, svc.ClientConnTimeout())

	// TLS Files
	m.GetPathReturnsOnCall(0, "client-key")
	require.Equal(t, "client-key", svc.TLSClientKeyFile())
	m.GetPathReturnsOnCall(1, "client-cert")
	require.Equal(t, "client-cert", svc.TLSClientCertFile())

	// Vault
	m.GetStringReturns("bad-cache-size")
	require.Equal(t, 100, svc.VaultTXStoreCacheSize())

	// MSP
	m.GetStringReturns("4")
	require.Equal(t, 4, svc.MSPCacheSize())
	m.GetStringReturns("bad")
	require.Equal(t, 3, svc.MSPCacheSize())

	// Ordering retries
	m.GetIntReturnsOnCall(0, 5)
	require.Equal(t, 5, svc.BroadcastNumRetries())
	m.GetIntReturnsOnCall(1, 0)
	require.Equal(t, 3, svc.BroadcastNumRetries())

	m.IsSetReturns(true)
	m.GetDurationReturns(100 * time.Millisecond)
	require.Equal(t, 100*time.Millisecond, svc.BroadcastRetryInterval())
	m.IsSetReturns(false)
	require.Equal(t, 500*time.Millisecond, svc.BroadcastRetryInterval())

	// Orderer connection pool
	m.IsSetReturns(true)
	m.GetIntReturns(20)
	require.Equal(t, 20, svc.OrdererConnectionPoolSize())
	m.IsSetReturns(false)
	require.Equal(t, 10, svc.OrdererConnectionPoolSize())

	defaultCh := svc.NewDefaultChannelConfig("new-ch")
	require.Equal(t, "new-ch", defaultCh.ID())
}

func TestService_MoreCases(t *testing.T) {
	m := &mock.Configuration{}

	t.Run("NewService_Errors", func(t *testing.T) {
		m := &mock.Configuration{}
		m.IsSetReturns(true)
		// Error in readItems (orderers)
		m.UnmarshalKeyReturnsOnCall(0, errors.New("orderer-err"))
		_, err := cfg.NewService(m, "mynet", false)
		require.Error(t, err)
		require.Contains(t, err.Error(), "orderer-err")

		// Error in readItems (peers)
		m.UnmarshalKeyReturnsOnCall(0, nil) // orderers ok
		m.UnmarshalKeyReturnsOnCall(1, errors.New("peer-err"))
		_, err = cfg.NewService(m, "mynet", false)
		require.Error(t, err)
		require.Contains(t, err.Error(), "peer-err")

		// Error in readItems (channels)
		m.UnmarshalKeyReturnsOnCall(0, nil) // orderers ok
		m.UnmarshalKeyReturnsOnCall(1, nil) // peers ok
		m.UnmarshalKeyReturnsOnCall(2, errors.New("channel-err"))
		_, err = cfg.NewService(m, "mynet", false)
		require.Error(t, err)
		require.Contains(t, err.Error(), "channel-err")

		// Error in createChannelMap (verify fails)
		m.UnmarshalKeyStub = func(key string, rawVal interface{}) error {
			if key == "fabric.mynet.channels" {
				p := rawVal.(*[]*cfg.Channel)
				*p = []*cfg.Channel{{Name: ""}} // invalid name
				return nil
			}
			return nil
		}
		_, err = cfg.NewService(m, "mynet", false)
		require.Error(t, err)
	})

	t.Run("MSPs_Resolvers", func(t *testing.T) {
		svc := &cfg.Service{Configuration: m}
		m.UnmarshalKeyReturns(errors.New("unmarshal-err"))
		_, err := svc.MSPs()
		require.Error(t, err)
		_, err = svc.Resolvers()
		require.Error(t, err)

		m.UnmarshalKeyReturns(nil)
		msps, err := svc.MSPs()
		require.NoError(t, err)
		require.Empty(t, msps)
		resolvers, err := svc.Resolvers()
		require.NoError(t, err)
		require.Empty(t, resolvers)
	})

	t.Run("PickPeer_Fallback", func(t *testing.T) {
		m := &mock.Configuration{}
		m.IsSetReturns(true)
		m.UnmarshalKeyStub = func(key string, rawVal interface{}) error {
			if key == "fabric.mynet.peers" {
				p := rawVal.(*[]*grpc.ConnectionConfig)
				*p = []*grpc.ConnectionConfig{
					{Address: "p1", Usage: ""},
					{Address: "p2", Usage: "UNKNOWN"}, // should log warning and be ignored for typed mapping
				}
				return nil
			}
			return nil
		}
		svc, err := cfg.NewService(m, "mynet", false)
		require.NoError(t, err)

		// should fallback to anything
		picked := svc.PickPeer(driver.PeerForQuery)
		require.NotNil(t, picked)
		require.Equal(t, "p1", picked.Address)
	})

	t.Run("Vault_TranslatePath", func(t *testing.T) {
		m := &mock.Configuration{}
		svc := &cfg.Service{Configuration: m}
		m.GetStringReturnsOnCall(0, "p1")
		require.Equal(t, sdriver.PersistenceName("p1"), svc.VaultPersistenceName())

		m.TranslatePathReturns("translated-path")
		require.Equal(t, "translated-path", svc.TranslatePath("original-path"))
	})

	t.Run("Channels", func(t *testing.T) {
		m := &mock.Configuration{}
		m.IsSetReturns(true)
		m.UnmarshalKeyStub = func(key string, rawVal interface{}) error {
			if key == "fabric.mynet.channels" {
				p := rawVal.(*[]*cfg.Channel)
				*p = []*cfg.Channel{
					{Name: "ch1", Default: true},
					{Name: "ch2", Quiet: true},
				}
				return nil
			}
			return nil
		}
		svc, err := cfg.NewService(m, "mynet", false)
		require.NoError(t, err)

		require.Equal(t, "ch1", svc.DefaultChannel())
		require.ElementsMatch(t, []string{"ch1", "ch2"}, svc.ChannelIDs())
		require.NotNil(t, svc.Channel("ch1"))
		require.NotNil(t, svc.Channel("ch2"))
		require.Nil(t, svc.Channel("unknown"))
		require.True(t, svc.IsChannelQuiet("ch2"))
		require.False(t, svc.IsChannelQuiet("ch1"))
		require.False(t, svc.IsChannelQuiet("unknown"))
	})
}
