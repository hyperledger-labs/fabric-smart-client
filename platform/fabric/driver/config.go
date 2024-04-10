/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
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

type ChannelConfig interface {
	ID() string
	FinalityWaitTimeout() time.Duration
	FinalityForPartiesWaitTimeout() time.Duration
	CommitterPollingTimeout() time.Duration
	CommitterFinalityNumRetries() int
	CommitterFinalityUnknownTXTimeout() time.Duration
	CommitterWaitForEventTimeout() time.Duration
	DeliverySleepAfterFailure() time.Duration
	ChaincodeConfigs() []ChaincodeConfig
	GetNumRetries() uint
	GetRetrySleep() time.Duration
	DiscoveryDefaultTTLS() time.Duration
	DiscoveryTimeout() time.Duration
}

type Configuration interface {
	// IsSet checks to see if the key has been set in any of the data locations
	IsSet(key string) bool
	// UnmarshalKey takes a single key and unmarshals it into a Struct
	UnmarshalKey(key string, rawVal interface{}) error
	// GetString returns the value associated with the key as a string
	GetString(key string) string

	TranslatePath(path string) string
}

type ConfigService interface {
	Configuration
	NetworkName() string
	DefaultChannel() string
	Channels() []ChannelConfig
	ChannelIDs() []string
	Orderers() []*grpc.ConnectionConfig
	SetConfigOrderers([]*grpc.ConnectionConfig) error
	PickOrderer() *grpc.ConnectionConfig
	BroadcastNumRetries() int
	BroadcastRetryInterval() time.Duration
	OrdererConnectionPoolSize() int
	PickPeer(funcType PeerFunctionType) *grpc.ConnectionConfig
	IsChannelQuite(name string) bool
	VaultPersistenceType() string
	VaultPersistencePrefix() string
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
