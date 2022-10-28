/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"time"
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
	ClientCert         File `yaml:"clientCert"`
	ClientKey          File `yaml:"clientKey"`
}

type ConnectionConfig struct {
	Address            string        `yaml:"address,omitempty"`
	ConnectionTimeout  time.Duration `yaml:"connectionTimeout,omitempty"`
	TLSEnabled         bool          `yaml:"tlsEnabled,omitempty"`
	TLSRootCertFile    string        `yaml:"tlsRootCertFile,omitempty"`
	TLSRootCertBytes   [][]byte      `yaml:"tlsRootCertBytes,omitempty"`
	ServerNameOverride string        `yaml:"serverNameOverride,omitempty"`
}

type Chaincode struct {
	Name    string `yaml:"name,omitempty"`
	Private bool   `yaml:"private,omitempty"`
}

type PvT struct {
	Enabled   bool   `yaml:"enabled,omitempty"`
	Chaincode string `yaml:"chaincode,omitempty"`
}

type Channel struct {
	Name       string        `yaml:"name,omitempty"`
	Default    bool          `yaml:"default,omitempty"`
	Quiet      bool          `yaml:"quiet,omitempty"`
	Pvt        PvT           `yaml:"pvt,omitempty"`
	NumRetries uint          `yaml:"NumRetries,omitempty"`
	RetrySleep time.Duration `yaml:"RetrySleep,omitempty"`
	Chaincodes []*Chaincode  `yaml:"chaincodes,omitempty"`
}

type Ordering struct {
	NumRetries    int           `yaml:"numRetries,omitempty"`
	RetryInterval time.Duration `yaml:"retryInterval,omitempty"`
}

type Network struct {
	Default      bool                `yaml:"default,omitempty"`
	MSPCacheSize int                 `yaml:"mspCacheSize"`
	DefaultMSP   string              `yaml:"defaultMSP"`
	MSPs         []*MSP              `yaml:"msps"`
	TLS          TLS                 `yaml:"tls"`
	Ordering     *Ordering           `yaml:"ordering"`
	Orderers     []*ConnectionConfig `yaml:"orderers"`
	Peers        []*ConnectionConfig `yaml:"peers"`
	Channels     []*Channel          `yaml:"channels"`
	Vault        Vault               `yaml:"vault"`
	Endpoint     *Endpoint           `yaml:"endpoint,omitempty"`
}
