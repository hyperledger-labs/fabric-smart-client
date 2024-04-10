/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

const (
	DefaultMSPCacheSize               = 3
	DefaultBroadcastNumRetries        = 3
	VaultPersistenceOptsKey           = "vault.persistence.opts"
	DefaultOrderingConnectionPoolSize = 10
	DefaultNumRetries                 = 3
	DefaultRetrySleep                 = 1 * time.Second
)

var logger = flogging.MustGetLogger("fabric-sdk.core.generic.config")

// configService models a configuration registry
type configService interface {
	// GetString returns the value associated with the key as a string
	GetString(key string) string
	// GetDuration returns the value associated with the key as a duration
	GetDuration(key string) time.Duration
	// GetBool returns the value associated with the key as a boolean
	GetBool(key string) bool
	// IsSet checks to see if the key has been set in any of the data locations
	IsSet(key string) bool
	// UnmarshalKey takes a single key and unmarshals it into a Struct
	UnmarshalKey(key string, rawVal interface{}) error
	// GetPath allows configuration strings that specify a (config-file) relative path
	GetPath(key string) string
	// TranslatePath translates the passed path relative to the config path
	TranslatePath(path string) string
	// GetInt returns the value associated with the key as an int
	GetInt(key string) int
}

type Service struct {
	name          string
	prefix        string
	configService configService

	configuredOrderers []*grpc.ConnectionConfig
	orderers           []*grpc.ConnectionConfig
	peerMapping        map[driver.PeerFunctionType][]*grpc.ConnectionConfig
	channels           []*Channel
	channelConfigs     []driver.ChannelConfig
	channelIDs         []string
	defaultChannel     string
}

func NewService(configService configService, name string, defaultConfig bool) (*Service, error) {
	// instantiate base service
	var service *Service
	if configService.IsSet("fabric." + name) {
		service = &Service{
			name:          name,
			prefix:        name + ".",
			configService: configService,
		}
	}
	if defaultConfig {
		service = &Service{
			name:          name,
			prefix:        "",
			configService: configService,
		}
	}

	// populate it

	// orderers
	var orderers []*grpc.ConnectionConfig
	if err := configService.UnmarshalKey("fabric."+service.prefix+"orderers", &orderers); err != nil {
		return nil, err
	}
	tlsEnabled := service.TLSEnabled()
	for _, v := range orderers {
		v.TLSEnabled = tlsEnabled
	}
	service.configuredOrderers = orderers
	service.orderers = orderers

	// peers
	var peers []*grpc.ConnectionConfig
	if err := configService.UnmarshalKey("fabric."+service.prefix+"peers", &peers); err != nil {
		return nil, err
	}

	peerMapping := map[driver.PeerFunctionType][]*grpc.ConnectionConfig{}
	for _, v := range peers {
		v.TLSEnabled = tlsEnabled
		if v.TLSDisabled {
			v.TLSEnabled = false
		}
		usage := strings.ToLower(v.Usage)
		switch {
		case len(usage) == 0:
			peerMapping[driver.PeerForAnything] = append(peerMapping[driver.PeerForAnything], v)
		case usage == "delivery":
			peerMapping[driver.PeerForDelivery] = append(peerMapping[driver.PeerForDelivery], v)
		case usage == "discovery":
			peerMapping[driver.PeerForDiscovery] = append(peerMapping[driver.PeerForDiscovery], v)
		case usage == "finality":
			peerMapping[driver.PeerForFinality] = append(peerMapping[driver.PeerForFinality], v)
		case usage == "query":
			peerMapping[driver.PeerForQuery] = append(peerMapping[driver.PeerForQuery], v)
		default:
			logger.Warn("connection usage [%s] not recognized [%v]", usage, v)
		}
	}
	service.peerMapping = peerMapping

	// channels
	var channels []*Channel
	var channelIDs []string
	var channelConfigs []driver.ChannelConfig
	if err := configService.UnmarshalKey("fabric."+service.prefix+"channels", &channels); err != nil {
		return nil, err
	}
	for _, channel := range channels {
		if err := channel.Verify(); err != nil {
			return nil, err
		}
		channelIDs = append(channelIDs, channel.Name)
		channelConfigs = append(channelConfigs, channel)
	}
	service.channels = channels
	service.channelIDs = channelIDs
	service.channelConfigs = channelConfigs
	for _, channel := range channels {
		if channel.Default {
			service.defaultChannel = channel.Name
			break
		}
	}

	return service, nil
}

func (s *Service) NetworkName() string {
	return s.name
}

func (s *Service) TLSEnabled() bool {
	return s.configService.GetBool("fabric." + s.prefix + "tls.enabled")
}

func (s *Service) TLSClientAuthRequired() bool {
	return s.configService.GetBool("fabric." + s.prefix + "tls.clientAuthRequired")
}

func (s *Service) TLSServerHostOverride() string {
	return s.configService.GetString("fabric." + s.prefix + "tls.serverhostoverride")
}

func (s *Service) ClientConnTimeout() time.Duration {
	return s.configService.GetDuration("fabric." + s.prefix + "client.connTimeout")
}

func (s *Service) TLSClientKeyFile() string {
	return s.configService.GetPath("fabric." + s.prefix + "tls.clientKey.file")
}

func (s *Service) TLSClientCertFile() string {
	return s.configService.GetPath("fabric." + s.prefix + "tls.clientCert.file")
}

func (s *Service) KeepAliveClientInterval() time.Duration {
	return s.configService.GetDuration("fabric." + s.prefix + "keepalive.interval")
}

func (s *Service) KeepAliveClientTimeout() time.Duration {
	return s.configService.GetDuration("fabric." + s.prefix + "keepalive.timeout")
}

func (s *Service) NewDefaultChannelConfig(name string) driver.ChannelConfig {
	return &Channel{
		Name:       name,
		Default:    false,
		Quiet:      false,
		NumRetries: DefaultNumRetries,
		RetrySleep: DefaultRetrySleep,
		Chaincodes: nil,
	}
}

func (s *Service) Orderers() []*grpc.ConnectionConfig {
	return s.orderers
}

func (s *Service) Peers() (map[driver.PeerFunctionType][]*grpc.ConnectionConfig, error) {
	var connectionConfigs []*grpc.ConnectionConfig
	if err := s.configService.UnmarshalKey("fabric."+s.prefix+"peers", &connectionConfigs); err != nil {
		return nil, err
	}

	res := map[driver.PeerFunctionType][]*grpc.ConnectionConfig{}
	for _, v := range connectionConfigs {
		v.TLSEnabled = s.TLSEnabled()
		if v.TLSDisabled {
			v.TLSEnabled = false
		}
		usage := strings.ToLower(v.Usage)
		switch {
		case len(usage) == 0:
			res[driver.PeerForAnything] = append(res[driver.PeerForAnything], v)
		case usage == "delivery":
			res[driver.PeerForDelivery] = append(res[driver.PeerForDelivery], v)
		case usage == "discovery":
			res[driver.PeerForDiscovery] = append(res[driver.PeerForDiscovery], v)
		case usage == "finality":
			res[driver.PeerForFinality] = append(res[driver.PeerForFinality], v)
		case usage == "query":
			res[driver.PeerForQuery] = append(res[driver.PeerForQuery], v)
		default:
			logger.Warn("connection usage [%s] not recognized [%v]", usage, v)
		}
	}
	return res, nil
}

func (s *Service) VaultPersistenceType() string {
	return s.configService.GetString("fabric." + s.prefix + "vault.persistence.type")
}

func (s *Service) VaultPersistencePrefix() string {
	return VaultPersistenceOptsKey
}

func (s *Service) VaultTXStoreCacheSize() int {
	defaultCacheSize := 100
	v := s.configService.GetString("fabric." + s.prefix + "vault.txidstore.cache.size")
	cacheSize, err := strconv.Atoi(v)
	if err != nil {
		return defaultCacheSize
	}

	if cacheSize < 0 {
		return defaultCacheSize
	}

	return cacheSize
}

// DefaultMSP returns the default MSP
func (s *Service) DefaultMSP() string {
	return s.configService.GetString("fabric." + s.prefix + "defaultMSP")
}

func (s *Service) MSPs() ([]MSP, error) {
	var confs []MSP
	if err := s.configService.UnmarshalKey("fabric."+s.prefix+"msps", &confs); err != nil {
		return nil, err
	}
	return confs, nil
}

// TranslatePath translates the passed path relative to the path from which the configuration has been loaded
func (s *Service) TranslatePath(path string) string {
	return s.configService.TranslatePath(path)
}

func (s *Service) DefaultChannel() string {
	return s.defaultChannel
}

func (s *Service) ChannelIDs() []string {
	return s.channelIDs
}

func (s *Service) Channels() []driver.ChannelConfig {
	return s.channelConfigs
}

func (s *Service) Resolvers() ([]Resolver, error) {
	var resolvers []Resolver
	if err := s.configService.UnmarshalKey("fabric."+s.prefix+"endpoint.resolvers", &resolvers); err != nil {
		return nil, err
	}
	return resolvers, nil
}

func (s *Service) GetString(key string) string {
	return s.configService.GetString("fabric." + s.prefix + key)
}

func (s *Service) GetDuration(key string) time.Duration {
	return s.configService.GetDuration("fabric." + s.prefix + key)
}

func (s *Service) GetBool(key string) bool {
	return s.configService.GetBool("fabric." + s.prefix + key)
}

func (s *Service) IsSet(key string) bool {
	return s.configService.IsSet("fabric." + s.prefix + key)
}

func (s *Service) UnmarshalKey(key string, rawVal interface{}) error {
	return s.configService.UnmarshalKey("fabric."+s.prefix+key, rawVal)
}

func (s *Service) GetPath(key string) string {
	return s.configService.GetPath("fabric." + s.prefix + key)
}

func (s *Service) MSPCacheSize() int {
	v := s.configService.GetString("fabric." + s.prefix + "mspCacheSize")
	if len(v) == 0 {
		return DefaultMSPCacheSize
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return DefaultMSPCacheSize
	}
	return i
}

func (s *Service) BroadcastNumRetries() int {
	v := s.configService.GetInt("fabric." + s.prefix + "ordering.numRetries")
	if v == 0 {
		return DefaultBroadcastNumRetries
	}
	return v
}

func (s *Service) BroadcastRetryInterval() time.Duration {
	return s.configService.GetDuration("fabric." + s.prefix + "ordering.retryInterval")
}

func (s *Service) OrdererConnectionPoolSize() int {
	k := "fabric." + s.prefix + "ordering.connectionPoolSize"
	if s.configService.IsSet(k) {
		return s.configService.GetInt(k)
	}
	return DefaultOrderingConnectionPoolSize
}

func (s *Service) SetConfigOrderers(orderers []*grpc.ConnectionConfig) error {
	s.orderers = append(s.orderers[:len(s.configuredOrderers)], orderers...)
	logger.Debugf("New Orderers [%d]", len(s.orderers))

	return nil
}

func (s *Service) PickOrderer() *grpc.ConnectionConfig {
	if len(s.orderers) == 0 {
		return nil
	}
	return s.orderers[rand.Intn(len(s.orderers))]
}

func (s *Service) PickPeer(ft driver.PeerFunctionType) *grpc.ConnectionConfig {
	source, ok := s.peerMapping[ft]
	if !ok {
		source = s.peerMapping[driver.PeerForAnything]
	}
	return source[rand.Intn(len(source))]
}

func (s *Service) IsChannelQuite(name string) bool {
	for _, chanDef := range s.channels {
		if chanDef.Name == name {
			return chanDef.Quiet
		}
	}
	return false
}
