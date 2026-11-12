/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package protos holds the protobuf messages of Fabric-x transactions (package fsc.fabricx.v1)
// and the Fabric-x transaction format, which its init function registers with
// fabric/protos.RegisterTransactionFormat under the driver name "fabricx". A Fabric-x
// transaction is exchanged and stored as a fabric/protos.TransactionEnvelope whose payload is
// a Transaction. Importing the package, as the Fabric-x transaction core does, makes the
// format available to every conversion in the process.
package protos

import (
	"encoding/json"

	"google.golang.org/protobuf/proto"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	fprotos "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/protos"
)

// Platform is the driver name of Fabric-x networks, as driver.ChannelKey.Platform holds it.
const Platform = "fabricx"

func init() {
	fprotos.RegisterTransactionFormat(Platform, fprotos.TransactionFormat{
		// The Fabric-x committer does not derive the txID from nonce and creator.
		SkipTxIDCheck: true,
		ToPayload:     toPayload,
		ToJSON:        toJSON,
	})
}

// txJSON has the JSON field names of a Fabric-x Transaction: the fields it shares with Fabric
// and the Key it carries at its top level.
type txJSON struct {
	fprotos.TxJSON
	Key *driver.ChannelKey `json:",omitempty"`
}

// deterministic marshals payloads, which are stored and compared.
var deterministic = proto.MarshalOptions{Deterministic: true}

// toPayload decodes raw, the JSON of a Fabric-x Transaction, strictly, and returns it as a
// marshaled Transaction with the key it carries.
func toPayload(raw []byte) ([]byte, *driver.ChannelKey, error) {
	var tx txJSON
	if err := fprotos.DecodeStrict(raw, &tx); err != nil {
		return nil, nil, errors.Wrap(err, "decode transaction JSON")
	}
	base, err := tx.ToMessage()
	if err != nil {
		return nil, nil, err
	}
	payload, err := deterministic.Marshal(&Transaction{Base: base})
	if err != nil {
		return nil, nil, errors.Wrap(err, "marshal transaction")
	}
	return payload, tx.Key, nil
}

// toJSON returns the JSON of the Fabric-x Transaction env carries, with a key only when env has
// a genesis hash. It fails as fprotos.TxJSONFromMessage does, including on a channel that
// differs from the signed proposal's, and on a payload without a base transaction.
func toJSON(env *fprotos.TransactionEnvelope) ([]byte, error) {
	msg := &Transaction{}
	if err := proto.Unmarshal(env.GetPayload(), msg); err != nil {
		return nil, errors.Wrap(err, "unmarshal transaction")
	}
	if msg.GetBase() == nil {
		return nil, errors.New("transaction envelope carries no transaction")
	}
	tx, err := fprotos.TxJSONFromMessage(msg.GetBase(), env)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(&txJSON{TxJSON: *tx, Key: fprotos.EnvelopeKey(env)})
	if err != nil {
		return nil, errors.Wrap(err, "marshal transaction JSON")
	}
	return raw, nil
}
