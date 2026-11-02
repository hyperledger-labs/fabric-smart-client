/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"encoding/json"
	"slices"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
)

// RequireChannelKey returns an error while the channel key of tx cannot be read.
// A view calls it before it sends tx to another node: the bytes of tx carry the
// key whenever it is readable, and a receiver of bytes without one builds the
// transaction on its default network instead. Once read, the key is cached, so
// the bytes produced afterwards carry it.
func RequireChannelKey(tx *fabric.Transaction) error {
	if _, err := tx.ChannelKey(); err != nil {
		return errors.WithMessagef(err, "cannot send tx [%s] of channel [%s] to another node", tx.ID(), tx.Channel())
	}
	return nil
}

// keyedBytes reads the ChannelKey that a serialized Fabric or Fabric-x transaction
// carries in its top-level Key field.
type keyedBytes struct {
	Key *driver.ChannelKey
}

// warnedLegacy records the senders already warned about sending transactions
// without a ChannelKey, so each is logged once.
// ponytail: grows with the number of distinct senders; bound it if a node talks to unbounded peers.
var warnedLegacy sync.Map

// routeReceived returns the name of the local network that serialized transaction
// raw belongs to, and the key raw carries (nil if none). It picks the network that
// has a configured channel matching the ChannelKey in raw, preferring the default
// network when several do, as when one network is configured under two names.
// Several matches none of which is the default network is an error.
//
// raw without a key comes from a node that does not send one; it is routed to the
// default network and a warning is logged once per sender. For bytes with a key,
// it returns an error if no local network matches; a candidate channel whose key
// cannot be read yet does not match, and the error says so.
func routeReceived(sp services.Provider, sender string, raw []byte) (string, *driver.ChannelKey, error) {
	var kb keyedBytes
	if err := json.Unmarshal(raw, &kb); err != nil {
		return "", nil, errors.Wrap(err, "failed decoding transaction")
	}
	if kb.Key == nil {
		if sender == "" {
			logger.Debugf("transaction carries no channel key; building it on the default network")
		} else if _, warned := warnedLegacy.LoadOrStore(sender, struct{}{}); !warned {
			logger.Warnf("transaction from [%s] carries no channel key; building it on the default network", sender)
		}
		return "", nil, nil
	}

	nsp, err := fabric.GetNetworkServiceProvider(sp)
	if err != nil {
		return "", nil, err
	}
	defaultName := nsp.DefaultName()
	var matches []string
	var unknown error
	for _, name := range nsp.Names() {
		fns, err := nsp.FabricNetworkService(name)
		if err != nil {
			unknown = errors.Join(unknown, errors.WithMessagef(err, "network [%s]", name))
			continue
		}
		// Only consult channels this node is configured for, so a sender cannot
		// make the node open a channel it does not use.
		conf := fns.ConfigService()
		if conf.DriverName() != kb.Key.Platform || !slices.Contains(conf.ChannelIDs(), kb.Key.Channel) {
			continue
		}
		ch, err := fns.Channel(kb.Key.Channel)
		if err != nil {
			unknown = errors.Join(unknown, errors.WithMessagef(err, "network [%s]", name))
			continue
		}
		key, err := ch.ChannelKey()
		if err != nil {
			unknown = errors.Join(unknown, errors.WithMessagef(err, "network [%s]: genesis hash unknown", name))
			continue
		}
		if !key.Matches(kb.Key) {
			continue
		}
		if fns.Name() == defaultName {
			return fns.Name(), kb.Key, nil
		}
		matches = append(matches, fns.Name())
	}
	switch len(matches) {
	case 1:
		return matches[0], kb.Key, nil
	case 0:
	default:
		return "", nil, errors.Errorf("networks %v all match channel [%s] and none is the default network", matches, kb.Key.Channel)
	}
	err = errors.Errorf("no local network matches channel [%s] with genesis hash [%x] on platform [%s]", kb.Key.Channel, kb.Key.GenesisHash, kb.Key.Platform)
	if unknown != nil {
		return "", nil, errors.Join(err, unknown)
	}
	return "", nil, err
}
