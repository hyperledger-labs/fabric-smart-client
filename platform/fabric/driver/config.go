/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

type ConfigService interface {
	// GetString returns the value associated with the key as a string
	GetString(key string) string
	// GetDuration returns the value associated with the key as a duration
	GetDuration(key string) time.Duration
	// GetBool returns the value associated with the key asa boolean
	GetBool(key string) bool
	// IsSet checks to see if the key has been set in any of the data locations
	IsSet(key string) bool
	// UnmarshalKey takes a single key and unmarshals it into a Struct
	UnmarshalKey(key string, rawVal interface{}) error
	// GetPath allows configuration strings that specify a (config-file) relative path
	GetPath(key string) string
	// TranslatePath translates the passed path relative to the config path
	TranslatePath(path string) string
}

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

// Config defines basic information the configuration should provide
type Config interface {
	// DefaultChannel returns the name of the default channel
	DefaultChannel() string

	// Channels return the list of registered channel names
	Channels() []string

	// Orderers returns the list of all registered ordereres
	Orderers() []*grpc.ConnectionConfig

	// Peers returns the list of all registered peers
	Peers() []*grpc.ConnectionConfig

	// PickPeer picks a peer at random among the peers that provide the passed functionality
	PickPeer(funcType PeerFunctionType) *grpc.ConnectionConfig
}
