/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

// PeerFunctionType defines classes of peers providing a specific functionality
type PeerFunctionType int

const (
	// PeerForAnything defines the class of peers that can be used for any function
	PeerForAnything = iota
	// PeerForDelivery defines the class of peers to be used for delivery
	PeerForDelivery
	// PeerForDiscovery defines the class of peers to be used for discovery
	PeerForDiscovery
	// PeerForFinality defines the class of peers to be used for finality
	PeerForFinality
	// PeerForQuery defines the class of peers to be used for query
	PeerForQuery
)

type ChaincodeConfig interface {
	ID() string
	IsPrivate() bool
}

type ListenerManagerProvider driver.ListenerManagerProvider[ValidationCode]

type ListenerManager driver.ListenerManager[ValidationCode]

type ChannelConfigProvider interface {
	GetChannelConfig(network, channel string) (ChannelConfig, error)
}

type ChannelConfig interface {
	ID() string
	FinalityWaitTimeout() time.Duration
	FinalityForPartiesWaitTimeout() time.Duration
	FinalityEventQueueWorkers() int
	CommitterPollingTimeout() time.Duration
	CommitterFinalityNumRetries() int
	CommitterFinalityUnknownTXTimeout() time.Duration
	CommitterWaitForEventTimeout() time.Duration
	DeliveryBufferSize() int
	DeliverySleepAfterFailure() time.Duration
	CommitParallelism() int
	ChaincodeConfigs() []ChaincodeConfig
	GetNumRetries() uint
	GetRetrySleep() time.Duration
	DiscoveryDefaultTTLS() time.Duration
	DiscoveryTimeout() time.Duration
}

type Configuration interface {
	// GetString returns the value associated with the key as a string
	GetString(key string) string
	// GetInt returns the value associated with the key as an integer
	GetInt(key string) int
	// GetDuration returns the value associated with the key as a duration
	GetDuration(key string) time.Duration
	// GetBool returns the value associated with the key asa boolean
	GetBool(key string) bool
	// GetStringSlice returns the value associated with the key as a slice of strings
	GetStringSlice(key string) []string
	// IsSet checks to see if the key has been set in any of the data locations
	IsSet(key string) bool
	// UnmarshalKey takes a single key and unmarshals it into a Struct
	UnmarshalKey(key string, rawVal interface{}) error
	// ConfigFileUsed returns the file used to populate the config registry
	ConfigFileUsed() string
	// GetPath allows configuration strings that specify a (config-file) relative path
	GetPath(key string) string
	// TranslatePath translates the passed path relative to the config path
	TranslatePath(path string) string
}

type ConfigService interface {
	Configuration
	NetworkName() string
	DriverName() string
	DefaultChannel() string
	Channel(name string) ChannelConfig
	ChannelIDs() []string
	Orderers() []*grpc.ConnectionConfig
	// OrderingTLSEnabled returns true, true if TLS is enabled because the key was set.
	// Default value is true.
	OrderingTLSEnabled() (bool, bool)
	// OrderingTLSClientAuthRequired returns true, true if TLS client-side authentication is enabled because the key was set.
	// Default value is false
	OrderingTLSClientAuthRequired() (bool, bool)
	SetConfigOrderers([]*grpc.ConnectionConfig) error
	PickOrderer() *grpc.ConnectionConfig
	BroadcastNumRetries() int
	BroadcastRetryInterval() time.Duration
	OrdererConnectionPoolSize() int
	PickPeer(funcType PeerFunctionType) *grpc.ConnectionConfig
	IsChannelQuiet(name string) bool
	VaultPersistenceName() driver2.PersistenceName
	VaultTXStoreCacheSize() int
	TLSServerHostOverride() string
	ClientConnTimeout() time.Duration
	TLSClientAuthRequired() bool
	TLSClientKeyFile() string
	TLSClientCertFile() string
	KeepAliveClientInterval() time.Duration
	KeepAliveClientTimeout() time.Duration
	NewDefaultChannelConfig(name string) ChannelConfig
	TLSEnabled() bool
}

type Resolver interface {
	// Name of the resolver
	Name() string
	// Domain is option
	Domain() string
	// Identity specifies an MSP Identity
	Identity() MSP
	// Addresses where to reach this identity
	Addresses() map[string]string
	// Aliases is a list of alias for this resolver
	Aliases() []string
}

type MSP interface {
	ID() string
	MSPType() string
	MSPID() string
	Path() string
	CacheSize() int
	Opts() map[interface{}]interface{}
}
