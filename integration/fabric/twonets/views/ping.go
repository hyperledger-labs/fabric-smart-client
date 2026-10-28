/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"
	"reflect"
	"sort"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// Ping checks that alice and bob see the same Fabric networks and that alice reaches
// every one of them. It exchanges a ping/pong with bob over FSC p2p, then queries
// the ledger of each network's default channel, a gRPC round trip to a peer of that
// network with that network's TLS settings.
type Ping struct{}

func (*Ping) Call(viewCtx view.Context) (any, error) {
	// Retrieve responder identity
	identityProvider, err := id.GetProvider(viewCtx)
	assert.NoError(err, "failed getting identity provider")
	responder := identityProvider.Identity("bob")

	// Open a session to the responder
	session, err := viewCtx.GetSession(viewCtx.Initiator(), responder)
	assert.NoError(err)
	// Send a ping and wait for the pong
	err = session.Send(viewCtx.Context(), []byte("ping"))
	assert.NoError(err)
	ch := session.Receive()
	var names []string
	select {
	case msg := <-ch:
		if msg.Status == view.ERROR {
			return nil, errors.New(string(msg.Payload))
		}
		assert.NoError(json.Unmarshal(msg.Payload, &names))
		sort.Strings(names)
		names2, err := fabric.GetFabricNetworkNames(viewCtx)
		assert.NoError(err)
		sort.Strings(names2)
		if len(names) == 0 || !reflect.DeepEqual(names, names2) {
			return nil, errors.Errorf("expected the same list of fabric networks, [%v]!=[%v]", names, names2)
		}
	case <-time.After(1 * time.Minute):
		return nil, errors.New("responder didn't pong in time")
	}

	for _, name := range names {
		_, channel, err := fabric.GetChannel(viewCtx, name, "")
		if err != nil {
			return nil, errors.Wrapf(err, "network [%s]", name)
		}
		if _, err := channel.Ledger().GetLedgerInfo(); err != nil {
			return nil, errors.Wrapf(err, "network [%s]: querying the ledger", name)
		}
	}

	return "OK", nil
}

type PingFactory struct{}

func (*PingFactory) NewView(_ []byte) (view.View, error) {
	return &Ping{}, nil
}
