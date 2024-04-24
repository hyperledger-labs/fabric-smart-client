/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/weaver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type RemoteGet struct {
	Network   string
	Channel   string
	Chaincode string
	Key       string
}

type RemoteGetView struct {
	*RemoteGet
}

func (g *RemoteGetView) Call(context view.Context) (interface{}, error) {
	// Get a weaver client to the relay of the given network
	fns, err := fabric.GetDefaultFNS(context)
	assert.NoError(err)
	relay := weaver.GetProvider(context).Relay(fns)

	// Build a query to the remote Fabric network.
	// Invoke the `Get` function on the passed key, on the passed chaincode deployed on the passed network and channel.
	query, err := relay.ToFabric().Query(
		fmt.Sprintf("fabric://%s.%s.%s/", g.Network, g.Channel, g.Chaincode),
		"Get", g.Key,
	)
	assert.NoError(err, "failed creating fabric query")

	// Perform the query
	res, err := query.Call()
	assert.NoError(err, "failed querying remote destination")
	assert.NotNil(res, "result should be non-empty")

	// Validate the proof accompanying the result
	proofRaw, err := res.Proof()
	assert.NoError(err, "failed getting proof from query result")
	proof, err := relay.ToFabric().ProofFromBytes(proofRaw)
	assert.NoError(err, "failed unmarshalling proof")
	assert.NoError(proof.Verify(), "failed verifying proof")

	// Inspect the content
	assert.Equal(res.Result(), proof.Result(), "result should be equal, got [%s]!=[%s]", string(res.Result()), string(proof.Result()))
	rwset1, err := res.RWSet()
	assert.NoError(err, "failed getting rwset from result")
	rwset2, err := proof.RWSet()
	assert.NoError(err, "failed getting rwset from proof")
	v1, err := rwset1.GetState(g.Chaincode, g.Key)
	assert.NoError(err, "failed getting key's value from rwset1")
	v2, err := rwset2.GetState(g.Chaincode, g.Key)
	assert.NoError(err, "failed getting key's value from rwset2")
	assert.Equal(v1, v2, "excepted same write [%s]!=[%s]", string(v1), string(v2))

	// return the value of the key, empty if not found
	return v1, nil
}

type RemoteGetViewFactory struct{}

func (p *RemoteGetViewFactory) NewView(in []byte) (view.View, error) {
	f := &RemoteGetView{}
	assert.NoError(json.Unmarshal(in, &f.RemoteGet))
	return f, nil
}
