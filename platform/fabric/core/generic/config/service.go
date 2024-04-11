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

	"github.com/pkg/errors"

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

type Service struct {
	driver.Configuration
	name   string
	prefix string

	configuredOrderers []*grpc.ConnectionConfig
	orderers           []*grpc.ConnectionConfig
	peerMapping        map[driver.PeerFunctionType][]*grpc.ConnectionConfig
	channels           []*Channel
	channelConfigs     []driver.ChannelConfig
	channelIDs         []string
	defaultChannel     string
}

func NewService(configService driver.Configuration, name string, defaultConfig bool) (*Service, error) {
	// instantiate base service
	var service *Service
	if configService.IsSet("fabric." + name) {
		service = &Service{
			name:          name,
			prefix:        name + ".",
			Configuration: configService,
		}
	} else {
		if defaultConfig {
			service = &Service{
				name:          name,
				prefix:        "",
				Configuration: configService,
			}
		} else {
			return nil, errors.Errorf("configuration for [%s] not found", name)
		}
	}
	// populate it
	if err := service.init(); err != nil {
		return nil, err
	}
	return service, nil
}

func (s *Service) init() error {
	// orderers
	var orderers []*grpc.ConnectionConfig
	if err := s.Configuration.UnmarshalKey("fabric."+s.prefix+"orderers", &orderers); err != nil {
		return err
	}
	tlsEnabled := s.TLSEnabled()
	for _, v := range orderers {
		v.TLSEnabled = tlsEnabled
	}
	s.configuredOrderers = orderers
	s.orderers = orderers

	// peers
	var peers []*grpc.ConnectionConfig
	if err := s.Configuration.UnmarshalKey("fabric."+s.prefix+"peers", &peers); err != nil {
		return err
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
	s.peerMapping = peerMapping

	// channels
	var channels []*Channel
	var channelIDs []string
	var channelConfigs []driver.ChannelConfig
	if err := s.Configuration.UnmarshalKey("fabric."+s.prefix+"channels", &channels); err != nil {
		return err
	}
	for _, channel := range channels {
		if err := channel.Verify(); err != nil {
			return err
		}
		channelIDs = append(channelIDs, channel.Name)
		channelConfigs = append(channelConfigs, channel)
	}
	s.channels = channels
	s.channelIDs = channelIDs
	s.channelConfigs = channelConfigs
	for _, channel := range channels {
		if channel.Default {
			s.defaultChannel = channel.Name
			break
		}
	}
	return nil
}

func (s *Service) NetworkName() string {
	return s.name
}

func (s *Service) TLSEnabled() bool {
	return s.Configuration.GetBool("fabric." + s.prefix + "tls.enabled")
}

func (s *Service) TLSClientAuthRequired() bool {
	return s.Configuration.GetBool("fabric." + s.prefix + "tls.clientAuthRequired")
}

func (s *Service) TLSServerHostOverride() string {
	return s.Configuration.GetString("fabric." + s.prefix + "tls.serverhostoverride")
}

func (s *Service) ClientConnTimeout() time.Duration {
	return s.Configuration.GetDuration("fabric." + s.prefix + "client.connTimeout")
}

func (s *Service) TLSClientKeyFile() string {
	return s.Configuration.GetPath("fabric." + s.prefix + "tls.clientKey.file")
}

func (s *Service) TLSClientCertFile() string {
	return s.Configuration.GetPath("fabric." + s.prefix + "tls.clientCert.file")
}

func (s *Service) KeepAliveClientInterval() time.Duration {
	return s.Configuration.GetDuration("fabric." + s.prefix + "keepalive.interval")
}

func (s *Service) KeepAliveClientTimeout() time.Duration {
	return s.Configuration.GetDuration("fabric." + s.prefix + "keepalive.timeout")
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

func (s *Service) VaultPersistenceType() string {
	return s.Configuration.GetString("fabric." + s.prefix + "vault.persistence.type")
}

func (s *Service) VaultPersistencePrefix() string {
	return VaultPersistenceOptsKey
}

func (s *Service) VaultTXStoreCacheSize() int {
	defaultCacheSize := 100
	v := s.Configuration.GetString("fabric." + s.prefix + "vault.txidstore.cache.size")
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
	return s.Configuration.GetString("fabric." + s.prefix + "defaultMSP")
}

func (s *Service) MSPs() ([]MSP, error) {
	var confs []MSP
	if err := s.Configuration.UnmarshalKey("fabric."+s.prefix+"msps", &confs); err != nil {
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
	return s.channelIDs
}

func (s *Service) Channels() []driver.ChannelConfig {
	return s.channelConfigs
}

func (s *Service) Resolvers() ([]Resolver, error) {
	var resolvers []Resolver
	if err := s.Configuration.UnmarshalKey("fabric."+s.prefix+"endpoint.resolvers", &resolvers); err != nil {
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
	v := s.Configuration.GetString("fabric." + s.prefix + "mspCacheSize")
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
	v := s.Configuration.GetInt("fabric." + s.prefix + "ordering.numRetries")
	if v == 0 {
		return DefaultBroadcastNumRetries
	}
	return v
}

func (s *Service) BroadcastRetryInterval() time.Duration {
	return s.Configuration.GetDuration("fabric." + s.prefix + "ordering.retryInterval")
}

func (s *Service) OrdererConnectionPoolSize() int {
	k := "fabric." + s.prefix + "ordering.connectionPoolSize"
	if s.Configuration.IsSet(k) {
		return s.Configuration.GetInt(k)
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
