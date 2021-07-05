/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"time"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Ping struct{}

func (p *Ping) Call(context view.Context) (interface{}, error) {
	// Retrieve responder identity
	responder := view2.GetIdentityProvider(context).Identity("bob")

	// Open a session to the responder
	session, err := context.GetSession(context.Initiator(), responder)
	assert.NoError(err) // Send a ping
	err = session.Send([]byte("ping"))
	assert.NoError(err) // Wait for the pong
	ch := session.Receive()
	select {
	case msg := <-ch:
		if msg.Status == view.ERROR {
			return nil, errors.New(string(msg.Payload))
		}
		var names []string
		assert.NoError(json.Unmarshal(msg.Payload, &names))
		sort.Strings(names)
		names2 := fabric.GetFabricNetworkNames(context)
		sort.Strings(names2)
		if len(names) == 0 || !reflect.DeepEqual(names, names2) {
			return nil, fmt.Errorf("exptectd the same list of fabric networks, [%v]!=[%v]", names, names2)
		}
	case <-time.After(1 * time.Minute):
		return nil, errors.New("responder didn't pong in time")
	}

	// Return
	return "OK", nil
}

type PingFactory struct{}

func (p *PingFactory) NewView(in []byte) (view.View, error) {
	return &Ping{}, nil
}
