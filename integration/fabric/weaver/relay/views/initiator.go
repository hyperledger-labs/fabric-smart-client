/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/weaver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/session"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type InitiatorView struct{}

func (p *InitiatorView) Call(context view.Context) (interface{}, error) {
	// Alice puts new data in `alpha` inside a given namespace of a given channel.
	// `alpha` is the default fabric network for Alice
	value := "sweet"
	_, err := fabric.GetDefaultChannel(context).Chaincode("ns1").Invoke(
		"Put", "pineapple", value,
	).Call()
	assert.NoError(err, "failed putting state")

	// Alice contacts Bob telling him new data is available in `alpha`
	session, err := session.NewJSON(context, context.Initiator(), view2.GetIdentityProvider(context).Identity("bob"))
	assert.NoError(err)
	assert.NoError(session.Send("pineapple"))

	var ack string
	assert.NoError(session.ReceiveWithTimeout(&ack, 1*time.Minute))
	assert.Equal("ack", ack, "failed getting ack back, got [%s]", ack)

	// Finally, Alice checks that Bob has actually stored the data she put in `alpha`, using `Weaver`.
	relay := weaver.GetProvider(context).Relay(fabric.GetDefaultFNS(context))
	query, err := relay.ToFabric().Query("fabric://beta.testchannel.ns2/", "Get", "pineapple")
	assert.NoError(err, "failed creating fabric query")
	res, err := query.Call()
	assert.NoError(err, "failed querying remote destination")
	assert.NotNil(res, "result should be non-empty")

	// Double-check the proof
	proofRaw, err := res.Proof()
	assert.NoError(err, "failed getting proof from query result")
	proof, err := relay.ToFabric().ProofFromBytes(proofRaw)
	assert.NoError(err, "failed unmarshalling proof")
	assert.NoError(proof.Verify(), "failed verifying proof")

	// Inspect content
	assert.Equal(res.Result(), proof.Result(), "result should be equal, got [%s]!=[%s]", string(res.Result()), string(proof.Result()))
	rwset1, err := res.RWSet()
	assert.NoError(err, "failed getting rwset from result")
	rwset2, err := proof.RWSet()
	assert.NoError(err, "failed getting rwset from proof")
	v1, err := rwset1.GetState("ns1", "pineapple")
	assert.NoError(err, "failed getting key's value from rwset1")
	v2, err := rwset2.GetState("ns1", "pineapple")
	assert.NoError(err, "failed getting key's value from rwset2")
	assert.Equal(v1, v2, "excepted same write [%s]!=[%s]", string(v1), string(v2))

	// Return
	return "OK", nil
}

type InitiatorViewFactory struct{}

func (p *InitiatorViewFactory) NewView(in []byte) (view.View, error) {
	return &InitiatorView{}, nil
}
