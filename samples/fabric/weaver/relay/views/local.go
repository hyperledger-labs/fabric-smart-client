/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type LocalPut struct {
	Chaincode string
	Key       string
	Value     string
}

type LocalPutView struct {
	*LocalPut
}

func (p *LocalPutView) Call(context view.Context) (interface{}, error) {
	// Invoke the passed chaincode to put the key/value pair
	txID, _, err := fabric.GetDefaultChannel(context).Chaincode(p.Chaincode).Invoke(
		"Put", p.Key, p.Value,
	).Call()
	assert.NoError(err, "failed to put key %s", p.Key)

	// return the transaction id
	return txID, nil
}

type LocalPutViewFactory struct{}

func (p *LocalPutViewFactory) NewView(in []byte) (view.View, error) {
	f := &LocalPutView{}
	assert.NoError(json.Unmarshal(in, &f.LocalPut))
	return f, nil
}

type LocalGet struct {
	Chaincode string
	Key       string
}

type LocalGetView struct {
	*LocalGet
}

func (g *LocalGetView) Call(context view.Context) (interface{}, error) {
	// Invoke the passed chaincode to get the value corresponding to the passed key
	v, err := fabric.GetDefaultChannel(context).Chaincode(g.Chaincode).Query(
		"Get", g.Key,
	).Call()
	assert.NoError(err, "failed to get key %s", g.Key)

	return v, nil
}

type LocalGetViewFactory struct{}

func (p *LocalGetViewFactory) NewView(in []byte) (view.View, error) {
	f := &LocalGetView{}
	assert.NoError(json.Unmarshal(in, &f.LocalGet))
	return f, nil
}
