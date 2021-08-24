/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/weaver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	session2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/session"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Responder struct{}

func (p *Responder) Call(context view.Context) (interface{}, error) {
	// Bob receives the name of the variable set by Alice.
	session := session2.JSON(context)
	var stateVariable string
	assert.NoError(session.ReceiveWithTimeout(&stateVariable, 60*time.Second), "failed getting state variable")

	// Bob `queries` Fabric network `alpha`, using `Weaver`, to get a proof that what Alice said is trustable.
	relay := weaver.GetProvider(context).Relay(fabric.GetDefaultFNS(context))
	query, err := relay.ToFabric().Query("fabric://alpha.testchannel.ns1/", "Get", stateVariable)
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
	v1, err := rwset1.GetState("ns1", stateVariable)
	assert.NoError(err, "failed getting key's value from rwset1")
	v2, err := rwset2.GetState("ns1", stateVariable)
	assert.NoError(err, "failed getting key's value from rwset2")
	assert.Equal(v1, v2, "excepted same write [%s]!=[%s]", string(v1), string(v2))

	// If the previous step is successful, then Bob stores the same data in `beta` (inside a given namespace of a given channel).
	_, err = fabric.GetDefaultChannel(context).Chaincode("ns2").Invoke(
		"Put", stateVariable, string(res.Result()),
	).Call()
	assert.NoError(err, "failed putting state")

	// Bob acks Alice that he has completed his task.
	assert.NoError(session.Send("ack"))

	return nil, nil
}
