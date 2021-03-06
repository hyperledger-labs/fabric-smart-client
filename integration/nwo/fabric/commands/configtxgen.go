/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commands

type OutputBlock struct {
	NetworkPrefix string
	ChannelID     string
	Profile       string
	ConfigPath    string
	OutputBlock   string
}

func (o OutputBlock) SessionName() string {
	return o.NetworkPrefix + "-configtxgen-output-block"
}

func (o OutputBlock) Args() []string {
	return []string{
		"-channelID", o.ChannelID,
		"-profile", o.Profile,
		"-configPath", o.ConfigPath,
		"-outputBlock", o.OutputBlock,
	}
}

type CreateChannelTx struct {
	NetworkPrefix         string
	ChannelID             string
	Profile               string
	ConfigPath            string
	OutputCreateChannelTx string
	BaseProfile           string
}

func (c CreateChannelTx) SessionName() string {
	return c.NetworkPrefix + "-configtxgen-create-channel-tx"
}

func (c CreateChannelTx) Args() []string {
	return []string{
		"-channelID", c.ChannelID,
		"-profile", c.Profile,
		"-configPath", c.ConfigPath,
		"-outputCreateChannelTx", c.OutputCreateChannelTx,
		"-channelCreateTxBaseProfile", c.BaseProfile,
	}
}

type OutputAnchorPeersUpdate struct {
	NetworkPrefix           string
	ChannelID               string
	Profile                 string
	ConfigPath              string
	AsOrg                   string
	OutputAnchorPeersUpdate string
}

func (o OutputAnchorPeersUpdate) SessionName() string {
	return o.NetworkPrefix + "-configtxgen-output-anchor-peers-update"
}

func (o OutputAnchorPeersUpdate) Args() []string {
	return []string{
		"-channelID", o.ChannelID,
		"-profile", o.Profile,
		"-configPath", o.ConfigPath,
		"-asOrg", o.AsOrg,
		"-outputAnchorPeersUpdate", o.OutputAnchorPeersUpdate,
	}
}
