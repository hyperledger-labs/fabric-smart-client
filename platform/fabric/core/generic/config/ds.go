/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"errors"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

type BCCSP struct {
	Default string            `yaml:"Default,omitempty"`
	SW      *SoftwareProvider `yaml:"SW,omitempty"`
	PKCS11  *PKCS11           `yaml:"PKCS11,omitempty"`
}

type SoftwareProvider struct {
	Hash     string `yaml:"Hash,omitempty"`
	Security int    `yaml:"Security,omitempty"`
}

type PKCS11 struct {
	// Default algorithms when not specified (Deprecated?)
	Security int    `yaml:"Security"`
	Hash     string `yaml:"Hash"`

	// PKCS11 options
	Library        string         `yaml:"Library"`
	Label          string         `yaml:"Label"`
	Pin            string         `yaml:"Pin"`
	SoftwareVerify bool           `yaml:"SoftwareVerify,omitempty"`
	Immutable      bool           `yaml:"Immutable,omitempty"`
	AltID          string         `yaml:"AltId,omitempty"`
	KeyIDs         []KeyIDMapping `yaml:"KeyIds,omitempty" mapstructure:"KeyIds"`
}

type KeyIDMapping struct {
	SKI string `yaml:"SKI,omitempty"`
	ID  string `yaml:"ID,omitempty"`
}

type MSPOpts struct {
	BCCSP *BCCSP `yaml:"BCCSP,omitempty"`
}

type MSP struct {
	ID        string                      `yaml:"id"`
	MSPType   string                      `yaml:"mspType"`
	MSPID     string                      `yaml:"mspID"`
	Path      string                      `yaml:"path"`
	CacheSize int                         `yaml:"cacheSize"`
	Opts      map[interface{}]interface{} `yaml:"opts, omitempty"`
}

type File struct {
	File string `yaml:"file"`
}

type Files struct {
	Files []string `yaml:"files"`
}

type TLS struct {
	Enabled            bool
	ClientAuthRequired bool
	Cert               File   `yaml:"cert"`
	Key                File   `yaml:"key"`
	ClientCert         File   `yaml:"clientCert"`
	ClientKey          File   `yaml:"clientKey"`
	RootCert           File   `yaml:"rootCert"`
	ClientRootCAs      Files  `yaml:"clientRootCAs"`
	RootCertFile       string `yaml:"rootCertFile"`
}

type ConnectionConfig = grpc.ConnectionConfig

type Chaincode struct {
	Name    string `yaml:"Name,omitempty"`
	Private bool   `yaml:"Private,omitempty"`
}

func (c Chaincode) ID() string {
	return c.Name
}

func (c Chaincode) IsPrivate() bool {
	return c.Private
}

type Finality struct {
	WaitForEventTimeout   time.Duration `yaml:"WaitForEventTimeout,omitempty"`
	ForPartiesWaitTimeout time.Duration `yaml:"ForPartiesWaitTimeout,omitempty"`
	EventQueueWorkers     int           `yaml:"EventQueueWorkers,omitempty"`
}

type Delivery struct {
	BufferSize          int           `yaml:"BufferSize,omitempty"`
	WaitForEventTimeout time.Duration `yaml:"WaitForEventTimeout,omitempty"`
	SleepAfterFailure   time.Duration `yaml:"SleepAfterFailure,omitempty"`
}

type Discovery struct {
	Timeout time.Duration `yaml:"Timeout,omitempty"`
}

type CommitterFinality struct {
	NumRetries       uint          `yaml:"NumRetries,omitempty"`
	UnknownTxTimeout time.Duration `yaml:"UnknownTxTimeout,omitempty"`
}

type Committer struct {
	WaitForEventTimeout time.Duration     `yaml:"WaitForEventTimeout,omitempty"`
	PollingTimeout      time.Duration     `yaml:"PollingTimeout,omitempty"`
	Finality            CommitterFinality `yaml:"Finality,omitempty"`
	Parallelism         int               `yaml:"Parallelism,omitempty"`
}

type Channel struct {
	Name       string        `yaml:"Name,omitempty"`
	Default    bool          `yaml:"Default,omitempty"`
	Quiet      bool          `yaml:"Quiet,omitempty"`
	NumRetries uint          `yaml:"NumRetries,omitempty"`
	RetrySleep time.Duration `yaml:"RetrySleep,omitempty"`
	Finality   Finality      `yaml:"Finality,omitempty"`
	Committer  Committer     `yaml:"Committer,omitempty"`
	Delivery   Delivery      `yaml:"Delivery,omitempty"`
	Discovery  Discovery     `yaml:"Discovery,omitempty"`
	Chaincodes []*Chaincode  `yaml:"Chaincodes,omitempty"`
}

func (c *Channel) Verify() error {
	if len(c.Name) == 0 {
		return errors.New("channel name is empty")
	}
	if c.NumRetries == 0 {
		c.NumRetries = 1
		logger.Warnf("channel configuration [%s], num retries set to 0", c.Name)
	}
	return nil
}

func (c *Channel) ID() string {
	return c.Name
}

func (c *Channel) DiscoveryDefaultTTLS() time.Duration {
	if c.Discovery.Timeout == 0 {
		return 5 * time.Minute
	}
	return c.Discovery.Timeout
}

func (c *Channel) CommitParallelism() int {
	return max(c.Committer.Parallelism, 1)
}

func (c *Channel) CommitterPollingTimeout() time.Duration {
	if c.Committer.PollingTimeout == 0 {
		return 100 * time.Millisecond
	}
	return c.Committer.PollingTimeout
}

func (c *Channel) DeliverySleepAfterFailure() time.Duration {
	if c.Delivery.SleepAfterFailure == 0 {
		return 10 * time.Second
	}
	return c.Delivery.SleepAfterFailure
}

func (c *Channel) ChaincodeConfigs() []driver.ChaincodeConfig {
	res := make([]driver.ChaincodeConfig, len(c.Chaincodes))
	for i, config := range c.Chaincodes {
		res[i] = config
	}
	return res
}

func (c *Channel) FinalityWaitTimeout() time.Duration {
	if c.Finality.WaitForEventTimeout == 0 {
		return 20 * time.Second
	}
	return c.Finality.WaitForEventTimeout
}

func (c *Channel) FinalityEventQueueWorkers() int {
	if c.Finality.EventQueueWorkers == 0 {
		return 1
	}
	return c.Finality.EventQueueWorkers
}

func (c *Channel) CommitterWaitForEventTimeout() time.Duration {
	if c.Committer.WaitForEventTimeout == 0 {
		return 300 * time.Second
	}
	return c.Committer.WaitForEventTimeout
}

func (c *Channel) DeliveryBufferSize() int {
	if c.Delivery.BufferSize <= 0 {
		return 1
	}
	return c.Delivery.BufferSize
}

func (c *Channel) DiscoveryTimeout() time.Duration {
	if c.Discovery.Timeout == 0 {
		return 20 * time.Second
	}
	return c.Discovery.Timeout
}

func (c *Channel) CommitterFinalityNumRetries() int {
	if c.Committer.Finality.NumRetries == 0 {
		return 3
	}
	return int(c.Committer.Finality.NumRetries)
}

func (c *Channel) CommitterFinalityUnknownTXTimeout() time.Duration {
	if c.Committer.Finality.UnknownTxTimeout == 0 {
		return 100 * time.Millisecond
	}
	return c.Discovery.Timeout
}

func (c *Channel) FinalityForPartiesWaitTimeout() time.Duration {
	if c.Finality.ForPartiesWaitTimeout == 0 {
		return 1 * time.Minute
	}
	return c.Finality.ForPartiesWaitTimeout
}

func (c *Channel) GetNumRetries() uint {
	return c.NumRetries
}

func (c *Channel) GetRetrySleep() time.Duration {
	return c.RetrySleep
}

type VaultOpts struct {
	Path string `yaml:"path"`
}

type VaultPersistence struct {
	Type string    `yaml:"type"`
	Opts VaultOpts `yaml:"opts"`
}

type Vault struct {
	Persistence VaultPersistence `yaml:"persistence"`
}

// Resolver models a Fabric identity resolver
type Resolver struct {
	// Name of the resolver
	Name string `yaml:"name,omitempty"`
	// Domain is option
	Domain string `yaml:"domain,omitempty"`
	// Identity specifies an MSP Identity
	Identity MSP `yaml:"identity,omitempty"`
	// Addresses where to reach this identity
	Addresses map[string]string `yaml:"addresses,omitempty"`
	// Aliases is a list of alias for this resolver
	Aliases []string `yaml:"aliases,omitempty"`
}

type Endpoint struct {
	Resolvers []Resolver `yaml:"resolvers,omitempty"`
}

type Network struct {
	Default    bool                `yaml:"default,omitempty"`
	DefaultMSP string              `yaml:"defaultMSP"`
	MSPs       []*MSP              `yaml:"msps"`
	TLS        TLS                 `yaml:"tls"`
	Orderers   []*ConnectionConfig `yaml:"orderers"`
	Peers      []*ConnectionConfig `yaml:"peers"`
	Channels   []*Channel          `yaml:"channels"`
	Vault      Vault               `yaml:"vault"`
	Endpoint   *Endpoint           `yaml:"endpoint,omitempty"`
}
