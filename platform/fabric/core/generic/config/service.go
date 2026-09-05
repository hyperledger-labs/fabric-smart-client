/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	sdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tlsconfig"
)

const (
	defaultMSPCacheSize               = 3
	defaultBroadcastNumRetries        = 3
	defaultBroadcastRetryInterval     = 500 * time.Millisecond
	defaultOrderingConnectionPoolSize = 10
	defaultNumRetries                 = 3
	defaultRetrySleep                 = 1 * time.Second
	defaultCacheSize                  = 100
	defaultConnectionTimeout          = 10 * time.Second

	GenericDriver = "generic"
)

var logger = logging.MustGetLogger()

var funcTypeMap = map[string]driver.PeerFunctionType{
	"":          driver.PeerForAnything,
	"delivery":  driver.PeerForDelivery,
	"discovery": driver.PeerForDiscovery,
	"finality":  driver.PeerForFinality,
	"query":     driver.PeerForQuery,
}

// Configuration is an alias for driver.Configuration
//
//go:generate counterfeiter -o mock/configuration.go -fake-name Configuration . Configuration
type Configuration = driver.Configuration

type Service struct {
	Configuration
	// networkTLS is the resolved client-side TLS of fabric.<network>.tls, the trust anchors
	// and credentials every connection this network dials inherits from.
	networkTLS grpc.SecureOptions

	name   string
	driver string
	prefix string

	configuredOrderers int
	orderers           []*ConnectionConfig
	peerMapping        map[driver.PeerFunctionType][]*ConnectionConfig
	channels           map[string]*Channel
	defaultChannel     string
}

// NewService creates a new Service instance by reading and translating the
// provided configuration. The function reads orderers, peers and channels
// using UnmarshalKey and performs minimal post-processing such as TLS path
// translation.
func NewService(configService Configuration, name string, defaultConfig bool) (*Service, error) {
	var prefix string
	if configService.IsSet("fabric." + name) {
		prefix = name + "."
	}
	if len(prefix) == 0 && !defaultConfig {
		return nil, errors.Errorf("configuration for [%s] not found", name)
	}
	driver := configService.GetString(fmt.Sprintf("fabric.%sdriver", prefix))
	if len(driver) == 0 {
		driver = GenericDriver
	}

	networkTLSKey := fmt.Sprintf("fabric.%stls", prefix)
	if err := tlsconfig.CheckRemovedKeys(configService, fmt.Sprintf("fabric.%s", prefix)); err != nil {
		return nil, err
	}
	networkTLS, err := tlsconfig.ResolveClient(configService, networkTLSKey)
	if err != nil {
		return nil, errors.WithMessagef(err, "invalid TLS configuration under %s", networkTLSKey)
	}

	orderers, err := readItems[*ConnectionConfig](configService, prefix, "orderers")
	if err != nil {
		return nil, err
	}
	if err := resolveEndpointTLS(configService, networkTLSKey, prefix, "orderers", orderers); err != nil {
		return nil, err
	}
	peers, err := readItems[*ConnectionConfig](configService, prefix, "peers")
	if err != nil {
		return nil, err
	}
	if err := resolveEndpointTLS(configService, networkTLSKey, prefix, "peers", peers); err != nil {
		return nil, err
	}
	peerMapping := createPeerMap(peers)

	channels, err := readItems[*Channel](configService, prefix, "channels")
	if err != nil {
		return nil, err
	}
	channelMap, defaultChannel, err := createChannelMap(channels)
	if err != nil {
		return nil, err
	}

	return &Service{
		Configuration:      configService,
		networkTLS:         networkTLS,
		name:               name,
		driver:             driver,
		prefix:             prefix,
		configuredOrderers: len(orderers),
		orderers:           orderers,
		peerMapping:        peerMapping,
		channels:           channelMap,
		defaultChannel:     defaultChannel,
	}, nil
}

// NetworkName returns the configured network name supplied to NewService.
func (s *Service) NetworkName() string {
	return s.name
}

// NetworkClientTLS returns the resolved client-side TLS of this network, which every
// connection it dials inherits from. Trust anchors discovered from a channel's MSPs are
// appended to a copy of ServerRootCAs by the caller that discovers them; they augment this
// pool rather than replacing it.
//
// It replaces OrderingTLSEnabled, OrderingTLSClientAuthRequired, TLSEnabled,
// TLSClientAuthRequired, TLSServerHostOverride, TLSClientKeyFile and TLSClientCertFile: seven
// accessors that each read one field of the same block, with the ordering.* pair shadowing
// two of them.
func (s *Service) NetworkClientTLS() grpc.SecureOptions {
	return s.networkTLS
}

// DriverName returns the selected driver name. When not set in the
// configuration, NewService initializes it to GenericDriver.
func (s *Service) DriverName() string {
	return s.driver
}

// ClientConnTimeout returns how long to wait when establishing a connection to this network.
func (s *Service) ClientConnTimeout() time.Duration {
	if !s.Configuration.IsSet("keepalive.connectionTimeout") {
		return defaultConnectionTimeout
	}
	return s.GetDuration("keepalive.connectionTimeout")
}

// ClientKeepAliveConfig return the client keep alive configuration.
// It returns nil, if no configuration was set.
// This functions loads and instance of grpc.ClientKeepAliveConfig
func (s *Service) ClientKeepAliveConfig() *grpc.ClientKeepAliveConfig {
	if !s.Configuration.IsSet("keepalive.interval") {
		return nil
	}
	c := &grpc.ClientKeepAliveConfig{}
	if err := s.UnmarshalKey("keepalive", c); err != nil {
		logger.Errorf("failed to unmarshal keepalive config [%s]", err)
		return nil
	}
	return c
}

func (s *Service) NewDefaultChannelConfig(name string) driver.ChannelConfig {
	return &Channel{
		Name:       name,
		Default:    false,
		Quiet:      false,
		NumRetries: defaultNumRetries,
		RetrySleep: defaultRetrySleep,
		Chaincodes: nil,
	}
}

func (s *Service) Orderers() []*ConnectionConfig {
	return s.orderers
}

func (s *Service) VaultPersistenceName() sdriver.PersistenceName {
	return sdriver.PersistenceName(s.GetString("vault.persistence"))
}

func (s *Service) VaultTXStoreCacheSize() int {
	if cacheSize, err := strconv.Atoi(s.GetString("vault.txidstore.cache.size")); err == nil && cacheSize >= 0 {
		return cacheSize
	}
	return defaultCacheSize
}

// DefaultMSP returns the default MSP identifier configured for this
// network (if any).
func (s *Service) DefaultMSP() string {
	return s.GetString("defaultMSP")
}

func (s *Service) MSPs() ([]MSP, error) {
	var confs []MSP
	if err := s.UnmarshalKey("msps", &confs); err != nil {
		return nil, err
	}
	return confs, nil
}

// TranslatePath translates the passed path relative to the path from which the configuration has been loaded
func (s *Service) TranslatePath(path string) string {
	return s.Configuration.TranslatePath(path)
}

func (s *Service) DefaultChannel() string {
	return s.defaultChannel
}

func (s *Service) ChannelIDs() []string {
	channelIDs := make([]string, len(s.channels))
	var i int
	for channelID := range s.channels {
		channelIDs[i] = channelID
		i++
	}
	return channelIDs
}

func (s *Service) Channel(name string) driver.ChannelConfig {
	return s.channels[name]
}

func (s *Service) Resolvers() ([]Resolver, error) {
	var resolvers []Resolver
	if err := s.UnmarshalKey("endpoint.resolvers", &resolvers); err != nil {
		return nil, err
	}
	return resolvers, nil
}

func (s *Service) GetString(key string) string {
	logger.Debugf("Get string [%s]", key)
	return s.Configuration.GetString("fabric." + s.prefix + key)
}

func (s *Service) GetInt(key string) int {
	return s.Configuration.GetInt("fabric." + s.prefix + key)
}

func (s *Service) GetDuration(key string) time.Duration {
	return s.Configuration.GetDuration("fabric." + s.prefix + key)
}

func (s *Service) GetBool(key string) bool {
	return s.Configuration.GetBool("fabric." + s.prefix + key)
}

func (s *Service) IsSet(key string) bool {
	return s.Configuration.IsSet("fabric." + s.prefix + key)
}

func (s *Service) UnmarshalKey(key string, rawVal any) error {
	return s.Configuration.UnmarshalKey("fabric."+s.prefix+key, rawVal)
}

func (s *Service) GetPath(key string) string {
	return s.Configuration.GetPath("fabric." + s.prefix + key)
}

// RawSubtree returns the raw map at the network-relative key. Overriding the embedded
// Configuration matters: without it a caller would silently read the unprefixed key and get
// another network's block, or none.
func (s *Service) RawSubtree(key string) (map[string]any, bool) {
	return s.Configuration.RawSubtree("fabric." + s.prefix + key)
}

// RawSubtrees returns the raw maps at the network-relative key when it holds an array of maps.
// Prefixed for the same reason as [Service.RawSubtree].
func (s *Service) RawSubtrees(key string) []map[string]any {
	return s.Configuration.RawSubtrees("fabric." + s.prefix + key)
}

func (s *Service) MSPCacheSize() int {
	if cacheSize, err := strconv.Atoi(s.GetString("mspCacheSize")); err == nil {
		return cacheSize
	}
	return defaultMSPCacheSize
}

func (s *Service) BroadcastNumRetries() int {
	if v := s.GetInt("ordering.numRetries"); v != 0 {
		return v
	}
	return defaultBroadcastNumRetries
}

func (s *Service) BroadcastRetryInterval() time.Duration {
	if s.IsSet("ordering.retryInterval") {
		return s.GetDuration("ordering.retryInterval")
	}
	return defaultBroadcastRetryInterval
}

func (s *Service) OrdererConnectionPoolSize() int {
	if s.IsSet("ordering.connectionPoolSize") {
		return s.GetInt("ordering.connectionPoolSize")
	}
	return defaultOrderingConnectionPoolSize
}

func (s *Service) SetConfigOrderers(orderers []*ConnectionConfig) error {
	s.orderers = append(s.orderers[:s.configuredOrderers], orderers...)
	logger.Debugf("New Orderers [%d]", len(s.orderers))

	return nil
}

func (s *Service) PickOrderer() *ConnectionConfig {
	if len(s.orderers) == 0 {
		return nil
	}
	return s.orderers[rand.Intn(len(s.orderers))]
}

func (s *Service) PickPeer(ft driver.PeerFunctionType) *ConnectionConfig {
	source, ok := s.peerMapping[ft]
	if !ok {
		source = s.peerMapping[driver.PeerForAnything]
	}
	if len(source) == 0 {
		return nil
	}
	return source[rand.Intn(len(source))]
}

func (s *Service) IsChannelQuiet(name string) bool {
	channel, ok := s.channels[name]
	return ok && channel.Quiet
}

func createChannelMap(channels []*Channel) (map[string]*Channel, string, error) {
	channelMap := make(map[string]*Channel, len(channels))
	var defaultChannel string
	for _, channel := range channels {
		if err := channel.Verify(); err != nil {
			return nil, "", err
		}
		channelMap[channel.Name] = channel
		if channel.Default {
			defaultChannel = channel.Name
		}
	}
	return channelMap, defaultChannel, nil
}

// resolveEndpointTLS resolves each endpoint's client-side TLS, inheriting per field from the
// network's tls block.
func resolveEndpointTLS(configService Configuration, networkTLSKey, prefix, key string, endpoints []*ConnectionConfig) error {
	resolved, err := tlsconfig.ResolveEndpointClients(configService, networkTLSKey,
		fmt.Sprintf("fabric.%s%s", prefix, key), len(endpoints))
	if err != nil {
		return err
	}
	for i, cc := range endpoints {
		cc.TLS = resolved[i]
	}
	return nil
}

func createPeerMap(peers []*ConnectionConfig) map[driver.PeerFunctionType][]*ConnectionConfig {
	peerMapping := map[driver.PeerFunctionType][]*ConnectionConfig{}
	for _, peerCC := range peers {
		if funcType, ok := funcTypeMap[strings.ToLower(peerCC.Usage)]; ok {
			peerMapping[funcType] = append(peerMapping[funcType], peerCC)
		} else {
			logger.Warnf("connection usage [%s] not recognized [%v]", peerCC.Usage, peerCC)
		}
	}
	return peerMapping
}

func readItems[T any](configService Configuration, prefix, key string) ([]T, error) {
	var items []T
	if err := configService.UnmarshalKey(fmt.Sprintf("fabric.%s%s", prefix, key), &items); err != nil {
		return nil, err
	}
	return items, nil
}

// The network config service must satisfy Source, or per-network TLS resolution needs an
// adapter. This is not automatic from the embedded Configuration: Service prefixes every key
// with fabric.<network>., and the unprefixed accessors would read the wrong block.
var _ tlsconfig.Source = (*Service)(nil)
