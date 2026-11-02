/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import "bytes"

// ChannelKey identifies a channel independently of the local name a node gives its
// network. Network names come from each node's own configuration and channel names
// need not be unique across networks, so a transaction sent to another node carries
// a ChannelKey for the receiver to find its local network with.
//
// Every member of a channel computes the same GenesisHash from its ledger, and two
// separate networks cannot share it.
type ChannelKey struct {
	// Platform is the driver name of the network, for example "generic" for
	// Fabric or "fabricx" for Fabric-x.
	Platform string `json:"platform"`
	// Channel is the channel name.
	Channel string `json:"channel"`
	// GenesisHash is the header hash of the channel's block 0.
	GenesisHash []byte `json:"genesisHash"`
}

// Matches reports whether k and other name the same channel of the same platform.
func (k *ChannelKey) Matches(other *ChannelKey) bool {
	return k.Platform == other.Platform && k.Channel == other.Channel && bytes.Equal(k.GenesisHash, other.GenesisHash)
}
