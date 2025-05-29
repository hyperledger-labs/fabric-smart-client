/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commands

type OSNAdminChannelList struct {
	NetworkPrefix  string
	OrdererAddress string
	CAFile         string
	ClientCert     string
	ClientKey      string
	ChannelID      string
}

func (c OSNAdminChannelList) SessionName() string {
	return c.NetworkPrefix + "osnadmin-channel-list"
}

func (c OSNAdminChannelList) Args() []string {
	args := []string{
		"channel", "list",
		"--no-status",
		"--orderer-address", c.OrdererAddress,
	}
	if c.CAFile != "" {
		args = append(args, "--ca-file", c.CAFile)
	}
	if c.ClientCert != "" {
		args = append(args, "--client-cert", c.ClientCert)
	}
	if c.ClientKey != "" {
		args = append(args, "--client-key", c.ClientKey)
	}
	if c.ChannelID != "" {
		args = append(args, "--channelID", c.ChannelID)
	}
	return args
}

type OSNAdminChannelJoin struct {
	NetworkPrefix  string
	OrdererAddress string
	CAFile         string
	ClientCert     string
	ClientKey      string
	ChannelID      string
	BlockPath      string
}

func (c OSNAdminChannelJoin) SessionName() string {
	return c.NetworkPrefix + "osnadmin-channel-join"
}

func (c OSNAdminChannelJoin) Args() []string {
	args := []string{
		"channel", "join",
		"--channelID", c.ChannelID,
		"--config-block", c.BlockPath,
		"-o", c.OrdererAddress,
	}
	if c.CAFile != "" {
		args = append(args, "--ca-file", c.CAFile)
	}
	if c.ClientCert != "" {
		args = append(args, "--client-cert", c.ClientCert)
	}
	if c.ClientKey != "" {
		args = append(args, "--client-key", c.ClientKey)
	}
	return args
}
