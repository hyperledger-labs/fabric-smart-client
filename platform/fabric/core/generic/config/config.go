/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import "time"

type MSP struct {
	ID      string `yaml:"id"`
	MSPType string `yaml:"mspType"`
	MSPID   string `yaml:"mspID"`
	Path    string `yaml:"path"`
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

type ConnectionConfig struct {
	Address            string        `yaml:"address,omitempty"`
	ConnectionTimeout  time.Duration `yaml:"connectionTimeout,omitempty"`
	TLSEnabled         bool          `yaml:"tlsEnabled,omitempty"`
	TLSRootCertFile    string        `yaml:"tlsRootCertFile,omitempty"`
	TLSRootCertBytes   [][]byte      `yaml:"tlsRootCertBytes,omitempty"`
	ServerNameOverride string        `yaml:"serverNameOverride,omitempty"`
}

type Channel struct {
	Name    string `yaml:"name"`
	Default bool   `yaml:"default,omitempty"`
}

type Network struct {
	Default       bool                `yaml:"default,omitempty"`
	BCCSP         *BCCSP              `yaml:"BCCSP,omitempty"`
	MSPConfigPath string              `yaml:"mspConfigPath,omitempty"`
	LocalMspId    string              `yaml:"localMspId,omitempty"`
	MSPs          []*MSP              `yaml:"msps"`
	TLS           TLS                 `yaml:"tls"`
	Orderers      []*ConnectionConfig `yaml:"orderers"`
	Peers         []*ConnectionConfig `yaml:"peers"`
	Channels      []*Channel          `yaml:"channels"`
	Vault         Vault               `yaml:"vault"`
	Endpoint      *Endpoint           `yaml:"endpoint,omitempty"`
}
