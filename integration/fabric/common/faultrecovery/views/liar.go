/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// LiarEndorseView creates an Asset and collects endorsements for it, expecting the
// responder to alter the transaction output and cause endorsement verification to fail.
// It returns an error naming the responder and the rejected transaction ID.
type LiarEndorseView struct {
	Submit
}

func (v *LiarEndorseView) Call(viewCtx view.Context) (any, error) {
	tx, err := state.NewTransaction(viewCtx)
	if err != nil {
		return nil, err
	}
	tx.SetNamespace(Namespace)
	if err := tx.AddOutput(&Asset{ID: tx.ID(), Value: v.Value}); err != nil {
		return nil, err
	}

	me := tx.FabricNetworkService().IdentityProvider().DefaultIdentity()
	if _, err := viewCtx.RunView(state.NewCollectEndorsementsView(tx, me, v.Responder)); err != nil {
		resolver, _ := endpoint.GetService(viewCtx).GetResolver(viewCtx.Context(), v.Responder)
		responderName := "bob"
		if resolver != nil && resolver.GetName() != "" {
			responderName = resolver.GetName()
		} else if len(v.Responder) > 0 && !strings.Contains(string(v.Responder), "-----BEGIN") {
			responderName = string(v.Responder)
		}
		return nil, errors.Wrapf(err, "tx [%s]: endorsement by [%s] failed", tx.ID(), responderName)
	}
	raw, err := tx.Bytes()
	if err != nil {
		return nil, err
	}
	return &Endorsed{Tx: raw, TxID: tx.ID()}, nil
}

// LiarEndorseViewFactory instantiates LiarEndorseView from JSON input.
type LiarEndorseViewFactory struct{}

func (*LiarEndorseViewFactory) NewView(in []byte) (view.View, error) {
	f := &LiarEndorseView{}
	if err := json.Unmarshal(in, &f.Submit); err != nil {
		return nil, err
	}
	return f, nil
}

// LiarResponderView alters the write value of the proposed transaction before
// endorsing it, simulating a malicious counterparty that attempts to tamper with the state.
type LiarResponderView struct{}

func (*LiarResponderView) Call(viewCtx view.Context) (any, error) {
	tx, err := state.ReceiveTransaction(viewCtx)
	if err != nil {
		return nil, err
	}
	asset := &Asset{ID: tx.ID(), Value: 999}
	_ = tx.GetOutputAt(0, asset)
	asset.Value++
	if err := tx.AddOutput(asset); err != nil {
		return nil, err
	}
	return viewCtx.RunView(state.NewEndorseView(tx))
}
