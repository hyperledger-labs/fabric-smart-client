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
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

const (
	defaultMSPCacheSize               = 3
	defaultBroadcastNumRetries        = 3
	defaultBroadcastRetryInterval     = 500 * time.Millisecond
	defaultOrderingConnectionPoolSize = 10
	defaultNumRetries                 = 3
	defaultRetrySleep                 = 1 * time.Second
	defaultCacheSize                  = 100

	defaultConnectionTimeout = 10 * time.Second
	defaultKeepaliveInterval = 60 * time.Second
	defaultKeepaliveTimeout  = 20 * time.Second

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

type Service struct {
	driver.Configuration
	name   string
	driver string
	prefix string

	configuredOrderers int
	orderers           []*ConnectionConfig
	peerMapping        map[driver.PeerFunctionType][]*ConnectionConfig
	channels           map[string]*Channel
	defaultChannel     string
}

func NewService(configService driver.Configuration, name string, defaultConfig bool) (*Service, error) {
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

	tlsEnabled := configService.GetBool(fmt.Sprintf("fabric.%stls.enabled", prefix))
	orderers, err := readItems[*ConnectionConfig](configService, prefix, "orderers")
	if err != nil {
		return nil, err
	}
	for _, v := range orderers {
		v.TLSEnabled = tlsEnabled
		if tlsEnabled && len(v.TLSRootCertFile) > 0 {
			v.TLSRootCertFile = configService.TranslatePath(v.TLSRootCertFile)
		}
	}
	peers, err := readItems[*ConnectionConfig](configService, prefix, "peers")
	if err != nil {
		return nil, err
	}
	peerMapping := createPeerMap(configService, peers, tlsEnabled)

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

func createPeerMap(configService driver.Configuration, peers []*ConnectionConfig, tlsEnabled bool) map[driver.PeerFunctionType][]*ConnectionConfig {
	peerMapping := map[driver.PeerFunctionType][]*ConnectionConfig{}
	for _, peerCC := range peers {
		peerCC.TLSEnabled = tlsEnabled && !peerCC.TLSDisabled
		if peerCC.TLSEnabled && len(peerCC.TLSRootCertFile) > 0 {
			peerCC.TLSRootCertFile = configService.TranslatePath(peerCC.TLSRootCertFile)
		}

		if funcType, ok := funcTypeMap[strings.ToLower(peerCC.Usage)]; ok {
			peerMapping[funcType] = append(peerMapping[funcType], peerCC)
		} else {
			logger.Warnf("connection usage [%s] not recognized [%v]", peerCC.Usage, peerCC)
		}
	}
	return peerMapping
}

func readItems[T any](configService driver.Configuration, prefix, key string) ([]T, error) {
	var items []T
	if err := configService.UnmarshalKey(fmt.Sprintf("fabric.%s%s", prefix, key), &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Service) NetworkName() string {
	return s.name
}

func (s *Service) OrderingTLSEnabled() (bool, bool) {
	if !s.Configuration.IsSet("ordering.tlsEnabled") {
		return true, false
	}
	return s.GetBool("ordering.tlsEnabled"), true
}

func (s *Service) OrderingTLSClientAuthRequired() (bool, bool) {
	if !s.Configuration.IsSet("ordering.tlsClientAuthRequired") {
		return false, false
	}
	return s.GetBool("ordering.tlsClientAuthRequired"), true
}

func (s *Service) DriverName() string {
	return s.driver
}

func (s *Service) TLSEnabled() bool {
	return s.GetBool("tls.enabled")
}

func (s *Service) TLSClientAuthRequired() bool {
	return s.GetBool("tls.clientAuthRequired")
}

func (s *Service) TLSServerHostOverride() string {
	return s.GetString("tls.serverhostoverride")
}

func (s *Service) ClientConnTimeout() time.Duration {
	if !s.Configuration.IsSet("keepalive.connectionTimeout") {
		return defaultConnectionTimeout
	}
	return s.GetDuration("keepalive.connectionTimeout")
}

func (s *Service) TLSClientKeyFile() string {
	return s.GetPath("tls.clientKey.file")
}

func (s *Service) TLSClientCertFile() string {
	return s.GetPath("tls.clientCert.file")
}

func (s *Service) KeepAliveClientInterval() time.Duration {
	if !s.Configuration.IsSet("keepalive.interval") {
		return defaultKeepaliveInterval
	}
	return s.GetDuration("keepalive.interval")
}

func (s *Service) KeepAliveClientTimeout() time.Duration {
	if !s.Configuration.IsSet("keepalive.timeout") {
		return defaultKeepaliveTimeout
	}
	return s.GetDuration("keepalive.timeout")
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

func (s *Service) VaultPersistenceName() driver2.PersistenceName {
	return driver2.PersistenceName(s.GetString("vault.persistence"))
}

func (s *Service) VaultTXStoreCacheSize() int {
	if cacheSize, err := strconv.Atoi(s.GetString("vault.txidstore.cache.size")); err == nil && cacheSize >= 0 {
		return cacheSize
	}
	return defaultCacheSize
}

// DefaultMSP returns the default MSP
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
	return s.Configuration.GetString("fabric." + s.prefix + key)
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

func (s *Service) UnmarshalKey(key string, rawVal interface{}) error {
	return s.Configuration.UnmarshalKey("fabric."+s.prefix+key, rawVal)
}

func (s *Service) GetPath(key string) string {
	return s.Configuration.GetPath("fabric." + s.prefix + key)
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
	return source[rand.Intn(len(source))]
}

func (s *Service) IsChannelQuiet(name string) bool {
	channel, ok := s.channels[name]
	return ok && channel.Quiet
}
