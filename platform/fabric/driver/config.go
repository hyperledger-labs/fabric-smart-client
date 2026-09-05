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
	UnmarshalKey(key string, rawVal any) error
	// ConfigFileUsed returns the file used to populate the config registry
	ConfigFileUsed() string
	// GetPath allows configuration strings that specify a (config-file) relative path
	GetPath(key string) string
	// TranslatePath translates the passed path relative to the config path
	TranslatePath(path string) string
	// RawSubtree returns the raw map at the given key, and reports whether the key names a
	// subtree. Used to decode a tls: block strictly, so an unknown key is an error rather
	// than a silent discard.
	RawSubtree(key string) (map[string]any, bool)
	// RawSubtrees returns the raw maps at the given key when it holds an array of maps, as
	// orderers and peers do. Array elements have no addressable key of their own.
	RawSubtrees(key string) []map[string]any
}

type ConfigService interface {
	Configuration
	NetworkName() string
	DriverName() string
	DefaultChannel() string
	Channel(name string) ChannelConfig
	ChannelIDs() []string
	Orderers() []*grpc.ConnectionConfig
	SetConfigOrderers([]*grpc.ConnectionConfig) error
	PickOrderer() *grpc.ConnectionConfig
	BroadcastNumRetries() int
	BroadcastRetryInterval() time.Duration
	OrdererConnectionPoolSize() int
	PickPeer(funcType PeerFunctionType) *grpc.ConnectionConfig
	IsChannelQuiet(name string) bool
	VaultPersistenceName() driver2.PersistenceName
	VaultTXStoreCacheSize() int
	ClientConnTimeout() time.Duration
	ClientKeepAliveConfig() *grpc.ClientKeepAliveConfig
	// NetworkClientTLS returns the resolved client-side TLS every connection this network
	// dials inherits from. Trust anchors discovered from a channel's MSPs are appended to a
	// copy of its ServerRootCAs; they augment the configured pool rather than replacing it.
	NetworkClientTLS() grpc.SecureOptions
	NewDefaultChannelConfig(name string) ChannelConfig
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
	Opts() map[string]any
}
