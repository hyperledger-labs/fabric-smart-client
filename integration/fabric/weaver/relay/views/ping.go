/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"time"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/weaver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Ping struct{}

func (p *Ping) Call(context view.Context) (interface{}, error) {
	// TODO: remove this line after the relay connections are working...
	//replacer.InteropFromContext(context, "ns2")

	// Alice puts a state in the namespace
	value := "sweet"
	_, _, err := fabric.GetDefaultChannel(context).Chaincode("ns1").Invoke(
		"Put", "pineapple", value,
	).Call()
	assert.NoError(err, "failed putting state")

	// Alice alerts Bob that the state is ready, and he can query it
	session, err := context.GetSession(context.Initiator(), view2.GetIdentityProvider(context).Identity("bob"))
	assert.NoError(err) // Send a ping
	err = session.Send([]byte("ping"))
	assert.NoError(err) // Wait for the pong
	ch := session.Receive()
	select {
	case msg := <-ch:
		if msg.Status == view.ERROR {
			return nil, errors.New(string(msg.Payload))
		}
		assert.Equal(value, string(msg.Payload), "expected response to be equal to value (%s), got [%s]", value, string(msg.Payload))
	case <-time.After(1 * time.Minute):
		return nil, errors.New("responder didn't pong in time")
	}

	// Query the state Bob has set
	relay := weaver.GetProvider(context).Relay(fabric.GetDefaultFNS(context))
	query, err := relay.ToFabric().Query("fabric://beta.testchannel.ns2/", "Get", "watermelon")
	assert.NoError(err, "failed creating fabric query")
	res, err := query.Call()
	assert.NoError(err, "failed querying remote destination")
	assert.NotNil(res, "result should be non-empty")

	// Double-check the proof
	proof, err := res.Proof()
	assert.NoError(err, "failed getting proof from query result")
	assert.NoError(relay.ToFabric().VerifyProof(proof), "failed verifying proof")

	// check the content of the result
	watermelonValue := res.Result()
	assert.NoError(err, "failed getting state [ns2.watermelon]")
	assert.Equal(string(watermelonValue), "red", "expected response to be equal to value, got [%v]", value)

	// Return
	return "OK", nil
}

type PingFactory struct{}

func (p *PingFactory) NewView(in []byte) (view.View, error) {
	return &Ping{}, nil
}
