/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commands

type Peers struct {
	NetworkPrefix string
	UserCert      string
	UserKey       string
	MSPID         string
	Server        string
	Channel       string
	ClientCert    string
	ClientKey     string
}

func (p Peers) SessionName() string {
	return p.NetworkPrefix + "-discover-peers"
}

func (p Peers) Args() []string {
	args := []string{
		"--userCert", p.UserCert,
		"--userKey", p.UserKey,
		"--msp", p.MSPID,
		"peers",
		"--server", p.Server,
		"--channel", p.Channel,
	}
	if p.ClientCert != "" {
		args = append(args, "--tlsCert", p.ClientCert)
	}
	if p.ClientKey != "" {
		args = append(args, "--tlsKey", p.ClientKey)
	}
	return args
}

type Config struct {
	UserCert   string
	UserKey    string
	MSPID      string
	Server     string
	Channel    string
	ClientCert string
	ClientKey  string
}

func (c Config) SessionName() string {
	return "discover-config"
}

func (c Config) Args() []string {
	args := []string{
		"--userCert", c.UserCert,
		"--userKey", c.UserKey,
		"--msp", c.MSPID,
		"config",
		"--server", c.Server,
		"--channel", c.Channel,
	}
	if c.ClientCert != "" {
		args = append(args, "--tlsCert", c.ClientCert)
	}
	if c.ClientKey != "" {
		args = append(args, "--tlsKey", c.ClientKey)
	}
	return args
}

type Endorsers struct {
	UserCert    string
	UserKey     string
	MSPID       string
	Server      string
	Channel     string
	Chaincode   string
	Chaincodes  []string
	Collection  string
	Collections []string
	ClientCert  string
	ClientKey   string
}

func (e Endorsers) SessionName() string {
	return "discover-endorsers"
}

func (e Endorsers) Args() []string {
	args := []string{
		"--userCert", e.UserCert,
		"--userKey", e.UserKey,
		"--msp", e.MSPID,
		"endorsers",
		"--server", e.Server,
		"--channel", e.Channel,
	}
	if e.ClientCert != "" {
		args = append(args, "--tlsCert", e.ClientCert)
	}
	if e.ClientKey != "" {
		args = append(args, "--tlsKey", e.ClientKey)
	}
	if e.Chaincode != "" {
		args = append(args, "--chaincode", e.Chaincode)
	}
	for _, cc := range e.Chaincodes {
		args = append(args, "--chaincode", cc)
	}
	if e.Collection != "" {
		args = append(args, "--collection", e.Collection)
	}
	for _, c := range e.Collections {
		args = append(args, "--collection", c)
	}
	return args
}
