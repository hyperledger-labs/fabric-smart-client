/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	cfg "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	sdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

func TestNewService_missingConfig(t *testing.T) {
	t.Parallel()
	m := &mock.Configuration{}
	m.IsSetReturns(false)
	// when defaultConfig=false and fabric.<name> is not set, error expected
	_, err := cfg.NewService(m, "mynet", false)
	require.Error(t, err)
}

// NewService must reject a network still carrying ordering.tlsEnabled. This pins the call
// site, not the check: the keys are relative to the network, so the prefix has to be prepended
// by CheckRemovedNetworkKeys — wiring up the node-scoped CheckRemovedKeys here instead compiles
// fine and silently matches nothing.
func TestNewService_rejectsRemovedOrderingTLSKey(t *testing.T) {
	t.Parallel()
	m := &mock.Configuration{}
	m.IsSetStub = func(key string) bool {
		// fabric.mynet present, plus the removed key the operator left behind.
		return key == "fabric.mynet" || key == "fabric.mynet.ordering.tlsenabled"
	}

	_, err := cfg.NewService(m, "mynet", false)
	require.ErrorContains(t, err, "fabric.mynet.ordering.tlsenabled")
	require.ErrorContains(t, err, "has been removed")
	require.ErrorContains(t, err, "fabric.mynet.tls.enabled")
}

func TestNewService_defaultsAndOrderers(t *testing.T) {
	t.Parallel()
	m := &mock.Configuration{}
	// simulate fabric.mynet present
	m.IsSetReturnsOnCall(0, true)   // called for fabric.mynet check
	m.GetStringReturnsOnCall(0, "") // fabric.mynetdriver -> default
	m.GetBoolReturns(true)

	// orderers: return a slice with one connection config. Their TLS is resolved from the
	// network block through tlsconfig now, not carried on flat fields.
	orderers := []*grpc.ConnectionConfig{{Address: "o:7050"}}
	m.UnmarshalKeyStub = func(key string, rawVal any) error {
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
	require.Len(t, svc.Orderers(), 1)
	// The network block is empty in this mock, so the endpoint resolves to TLS off rather
	// than to a translated path.
	require.False(t, svc.Orderers()[0].TLS.UseTLS)
	require.Equal(t, "ch1", svc.DefaultChannel())
}

func TestClientKeepAliveConfig_UnmarshalError(t *testing.T) {
	t.Parallel()
	m := &mock.Configuration{}
	m.IsSetStub = setsEverythingButRemovedKeys // keepalive.interval is set
	m.UnmarshalKeyReturnsOnCall(0, errors.New("boom"))

	svc := &cfg.Service{Configuration: m}
	// should return nil on unmarshal error
	k := svc.ClientKeepAliveConfig()
	require.Nil(t, k)
}

func TestVaultAndMSPSettings(t *testing.T) {
	t.Parallel()
	m := &mock.Configuration{}
	// fabric prefix empty
	m.GetStringReturnsOnCall(0, "persistenceName") // vault.persistence
	m.GetStringReturnsOnCall(1, "50")              // vault.txidstore.cache.size (string that parses)
	m.GetStringReturnsOnCall(2, "defaultMSP")      // defaultMSP
	m.UnmarshalKeyStub = func(key string, rawVal any) error {
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
	t.Parallel()
	ch := &cfg.Channel{}
	// default values
	require.Equal(t, time.Duration(5*time.Minute), ch.DiscoveryDefaultTTLS())
	require.Equal(t, 1, ch.CommitParallelism())
	require.Equal(t, 1*time.Second, ch.CommitterPollingTimeout())
	require.Equal(t, 10*time.Second, ch.DeliverySleepAfterFailure())
	require.Equal(t, 20*time.Second, ch.FinalityWaitTimeout())
	require.Equal(t, 1, ch.FinalityEventQueueWorkers())
	require.Equal(t, 300*time.Second, ch.CommitterWaitForEventTimeout())
	require.Equal(t, 1, ch.DeliveryBufferSize())
	require.Equal(t, 20*time.Second, ch.DiscoveryTimeout())
	require.Equal(t, 3, ch.CommitterFinalityNumRetries())
	require.Equal(t, time.Duration(100*time.Millisecond), ch.CommitterFinalityUnknownTXTimeout())
	require.Equal(t, time.Minute, ch.FinalityForPartiesWaitTimeout())

	// test PollingTimeout clamping
	require.Equal(t, time.Millisecond, (&cfg.Channel{Committer: cfg.Committer{PollingTimeout: -5 * time.Millisecond}}).CommitterPollingTimeout())
	require.Equal(t, time.Millisecond, (&cfg.Channel{Committer: cfg.Committer{PollingTimeout: 500 * time.Microsecond}}).CommitterPollingTimeout())

	// ChaincodeConfigs should convert to driver.ChaincodeConfig
	cc := &cfg.Chaincode{Name: "cc1"}
	c := &cfg.Channel{Chaincodes: []*cfg.Chaincode{cc}}
	arr := c.ChaincodeConfigs()
	require.Len(t, arr, 1)
	require.Equal(t, "cc1", arr[0].ID())
}

// CommitterFinalityUnknownTXTimeout must return its own configured field and
// never Discovery.Timeout. The two are set to different non-zero values below so
// that returning the wrong field cannot pass, and so that neither value can be
// mistaken for the other's default.
func TestCommitterFinalityUnknownTXTimeout(t *testing.T) {
	t.Parallel()

	t.Run("returns the configured value, not the discovery timeout", func(t *testing.T) {
		t.Parallel()
		ch := &cfg.Channel{}
		ch.Committer.Finality.UnknownTxTimeout = 1 * time.Second
		ch.Discovery.Timeout = 5 * time.Minute

		require.Equal(t, 1*time.Second, ch.CommitterFinalityUnknownTXTimeout())
	})

	t.Run("does not fall back to an unset discovery timeout", func(t *testing.T) {
		t.Parallel()
		ch := &cfg.Channel{}
		ch.Committer.Finality.UnknownTxTimeout = 1 * time.Second
		// Discovery.Timeout deliberately left unset.

		require.Equal(t, 1*time.Second, ch.CommitterFinalityUnknownTXTimeout())
	})

	t.Run("defaults to 100ms when unset, whatever the discovery timeout", func(t *testing.T) {
		t.Parallel()
		ch := &cfg.Channel{}
		ch.Discovery.Timeout = 5 * time.Minute

		require.Equal(t, 100*time.Millisecond, ch.CommitterFinalityUnknownTXTimeout())
	})
}

func TestCreatePeerMapAndPickPeer(t *testing.T) {
	t.Parallel()
	m := &mock.Configuration{}
	m.IsSetReturnsOnCall(0, true) // fabric.network present for NewService
	m.GetStringReturnsOnCall(0, "")
	m.GetBoolReturns(true) // TLS enabled
	m.UnmarshalKeyStub = func(key string, rawVal any) error {
		switch key {
		case "fabric.test.peers", "fabric.testpeers":
			p, ok := rawVal.(*[]*grpc.ConnectionConfig)
			if !ok {
				return nil
			}
			*p = []*grpc.ConnectionConfig{
				{Address: "p1", Usage: "query"},
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
	t.Parallel()
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
	t.Parallel()
	m := &mock.Configuration{}
	// Initialize with some basic setup to avoid NewService errors
	m.IsSetStub = setsEverythingButRemovedKeys
	m.GetStringReturns("")
	svc, err := cfg.NewService(m, "mynet", true)
	require.NoError(t, err)

	// Reset mock for subsequent calls to have more control
	m.IsSetReturns(false)
	m.GetBoolReturns(false)
	m.GetStringReturns("")

	// The seven TLS accessors that used to live here — OrderingTLSEnabled,
	// OrderingTLSClientAuthRequired, TLSEnabled, TLSClientAuthRequired,
	// TLSServerHostOverride, TLSClientKeyFile and TLSClientCertFile — are replaced by one
	// resolved value. With an empty network block it resolves to TLS off.
	require.False(t, svc.NetworkClientTLS().UseTLS)

	// Keepalive
	m.IsSetReturns(false)
	require.Equal(t, 10*time.Second, svc.ClientConnTimeout())

	m.IsSetStub = setsEverythingButRemovedKeys
	m.GetDurationReturns(5 * time.Second)
	require.Equal(t, 5*time.Second, svc.ClientConnTimeout())

	// Vault
	m.GetStringReturns("bad-cache-size")
	require.Equal(t, 100, svc.VaultTXStoreCacheSize())

	// MSP
	m.GetStringReturns("4")
	require.Equal(t, 4, svc.MSPCacheSize())
	m.GetStringReturns("bad")
	require.Equal(t, 3, svc.MSPCacheSize())

	// Ordering retries
	getIntCallsBefore := m.GetIntCallCount()
	m.GetIntReturnsOnCall(getIntCallsBefore, 5)
	require.Equal(t, 5, svc.BroadcastNumRetries())
	require.Equal(t, "fabric.mynet.ordering.numRetries", m.GetIntArgsForCall(getIntCallsBefore))
	m.GetIntReturnsOnCall(getIntCallsBefore+1, 0)
	require.Equal(t, 3, svc.BroadcastNumRetries())

	m.IsSetStub = setsEverythingButRemovedKeys
	m.GetDurationReturns(100 * time.Millisecond)
	require.Equal(t, 100*time.Millisecond, svc.BroadcastRetryInterval())
	m.IsSetReturns(false)
	require.Equal(t, 500*time.Millisecond, svc.BroadcastRetryInterval())

	// Orderer connection pool
	m.IsSetStub = setsEverythingButRemovedKeys
	m.GetIntReturns(20)
	poolCallIndex := m.GetIntCallCount()
	require.Equal(t, 20, svc.OrdererConnectionPoolSize())
	require.Equal(t, "fabric.mynet.ordering.connectionPoolSize", m.GetIntArgsForCall(poolCallIndex))
	m.IsSetReturns(false)
	require.Equal(t, 10, svc.OrdererConnectionPoolSize())

	defaultCh := svc.NewDefaultChannelConfig("new-ch")
	require.Equal(t, "new-ch", defaultCh.ID())
}

func TestService_MoreCases(t *testing.T) {
	t.Parallel()
	m := &mock.Configuration{}

	t.Run("NewService_Errors", func(t *testing.T) {
		t.Parallel()
		m := &mock.Configuration{}
		m.IsSetStub = setsEverythingButRemovedKeys
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
		m.UnmarshalKeyStub = func(key string, rawVal any) error {
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
		t.Parallel()
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
		t.Parallel()
		m := &mock.Configuration{}
		m.IsSetStub = setsEverythingButRemovedKeys
		m.UnmarshalKeyStub = func(key string, rawVal any) error {
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
		t.Parallel()
		m := &mock.Configuration{}
		svc := &cfg.Service{Configuration: m}
		m.GetStringReturnsOnCall(0, "p1")
		require.Equal(t, sdriver.PersistenceName("p1"), svc.VaultPersistenceName())

		m.TranslatePathReturns("translated-path")
		require.Equal(t, "translated-path", svc.TranslatePath("original-path"))
	})

	t.Run("Channels", func(t *testing.T) {
		t.Parallel()
		m := &mock.Configuration{}
		m.IsSetStub = setsEverythingButRemovedKeys
		m.UnmarshalKeyStub = func(key string, rawVal any) error {
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

// setsEverythingButRemovedKeys answers true for any key except the ones the TLS migration
// removed. A blanket IsSet -> true would claim fabric.<net>.ordering.tlsEnabled is configured,
// and NewService rejects a network still carrying it — correctly, so the mock has to be
// specific rather than the check made lenient.
func setsEverythingButRemovedKeys(key string) bool {
	switch strings.ToLower(key) {
	case "fabric.mynet.ordering.tlsenabled", "fabric.mynet.ordering.tlsclientauthrequired",
		"fabric.ordering.tlsenabled", "fabric.ordering.tlsclientauthrequired",
		"fabric.network.ordering.tlsenabled", "fabric.network.ordering.tlsclientauthrequired":
		return false
	}
	return true
}
