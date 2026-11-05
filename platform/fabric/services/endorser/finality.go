/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/protos"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Finality struct {
	TxID    cdriver.TxID
	Network cdriver.Network
	Channel cdriver.Channel
}

type finalityView struct {
	*Finality
	timeout time.Duration
	// key is the channel key of a proto.FinalityRequest input that carried one; Call
	// resolves it into Network before using it. A JSON Finality input or a request
	// with an empty genesis hash leaves it nil, so Network (its own or "", the
	// default network) is used unchanged.
	key *driver.ChannelKey
}

func (f *finalityView) Call(viewCtx view.Context) (any, error) {
	if f.key != nil {
		network, err := routeKey(viewCtx, f.key)
		if err != nil {
			return nil, err
		}
		f.Network = network
	}
	fns, err := fabric.GetFabricNetworkService(viewCtx, f.Network)
	if err != nil {
		return nil, errors.WithMessagef(err, "fabric network service [%s] not found", f.Network)
	}
	ch, err := fns.Channel(f.Channel)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed getting channel [%s:%s]", f.Network, f.Channel)
	}
	c := viewCtx.Context()
	if f.timeout != 0 {
		var cancel context.CancelFunc
		c, cancel = context.WithTimeout(c, f.timeout)
		defer cancel()
	}
	return nil, ch.Finality().IsFinal(c, f.TxID)
}

func NewFinalityView(tx *Transaction) *finalityView {
	return &finalityView{Finality: &Finality{TxID: tx.ID(), Network: tx.Network(), Channel: tx.Channel()}}
}

// NewFinalityWithTimeoutView runs the finality view for the passed transaction and timeout
func NewFinalityWithTimeoutView(tx *Transaction, timeout time.Duration) *finalityView {
	return &finalityView{Finality: &Finality{TxID: tx.ID(), Network: tx.Network(), Channel: tx.Channel()}, timeout: timeout}
}

type FinalityViewFactory struct{}

// NewView builds a finality view from in, which is either the JSON encoding of
// Finality or a protos.FinalityRequest: in whose first byte is '{', or empty in,
// is decoded as Finality, and any other in is unmarshaled as the proto. JSON with
// leading whitespace is not accepted: the first byte of a marshaled
// FinalityRequest, 0x0a (the tag of tx_id), is also '\n', and a request whose
// transaction ID the client chooses can be valid JSON after it, so no rule that
// skips whitespace tells the encodings apart. A client sends the JSON of
// Finality without leading whitespace, as json.Marshal gives it. in is a view input
// from a client, not a node-to-node message, so it is not session-versioned.
//
// A proto request with a channel genesis hash carries a channel key; Call
// resolves the key into the local network at run time with routeKey, since the
// factory itself is not given a services.Provider to resolve it with. A request
// with an empty genesis hash has no key, so the view runs on the default
// network.
func (*FinalityViewFactory) NewView(in []byte) (view.View, error) {
	f := &finalityView{Finality: &Finality{}}
	if len(in) == 0 || in[0] == '{' {
		if err := json.Unmarshal(in, f.Finality); err != nil {
			return nil, errors.Wrap(err, "failed unmarshalling input")
		}
		return f, nil
	}
	req := &protos.FinalityRequest{}
	if err := proto.Unmarshal(in, req); err != nil {
		return nil, errors.Wrap(err, "failed unmarshalling input")
	}
	f.TxID = req.GetTxId()
	f.Channel = req.GetChannel()
	if len(req.GetChannelGenesisHash()) > 0 {
		if req.GetPlatform() == "" {
			return nil, errors.New("finality request with a channel genesis hash names no platform")
		}
		f.key = &driver.ChannelKey{Platform: req.GetPlatform(), Channel: req.GetChannel(), GenesisHash: req.GetChannelGenesisHash()}
	}
	return f, nil
}
