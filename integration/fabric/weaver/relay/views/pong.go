/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"fmt"
	"time"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/weaver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Pong struct{}

func (p *Pong) Call(context view.Context) (interface{}, error) {
	// Retrieve the session opened by Alice
	session := context.Session()

	// Read the message from Alice
	ch := session.Receive()
	var payload []byte
	select {
	case msg := <-ch:
		payload = msg.Payload
	case <-time.After(60 * time.Second):
		return nil, errors.New("time out reached")
	}

	// Respond with a pong if a ping is received, an error otherwise
	m := string(payload)

	switch {
	case m != "ping":
		// reply with an error
		err := session.SendError([]byte(fmt.Sprintf("expected ping, got %s", m)))
		assert.NoError(err)
		return nil, fmt.Errorf("expected ping, got %s", m)
	default:
		// Bob puts a state in the namespace
		_, _, err := fabric.GetDefaultChannel(context).Chaincode("ns2").Invoke(
			"Put", "watermelon", "red",
		).Call()
		assert.NoError(err, "failed putting state")

		// Query the state Alice has set
		relay := weaver.GetProvider(context).Relay(fabric.GetDefaultFNS(context))
		query, err := relay.ToFabric().Query("fabric://alpha.testchannel.ns1/", "Get", "pineapple")
		assert.NoError(err, "failed creating fabric query")
		res, err := query.Call()
		assert.NoError(err, "failed querying remote destination")
		assert.NotNil(res, "result should be non-empty")

		// Double-check the proof
		proof, err := res.Proof()
		assert.NoError(err, "failed getting proof from query result")
		assert.NoError(relay.ToFabric().VerifyProof(proof), "failed verifying proof")

		value := res.Result()

		// send back the value read
		assert.NoError(session.Send(value))
	}

	// Return
	return "OK", nil
}
