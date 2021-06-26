/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/test-go/testify/assert"
)

type House struct {
	Address   string
	Valuation uint64
	LinearID  string
	Owner     view.Identity
}

func (h *House) SetLinearID(id string) string {
	if len(h.LinearID) == 0 {
		h.LinearID = id
	}
	return h.LinearID
}

func (h *House) Owners() Identities {
	return []view.Identity{h.Owner}
}

type Asset struct {
	ObjectType        string        `json:"objectType"`
	ID                string        `json:"assetID"`
	Owner             view.Identity `json:"owner"`
	PublicDescription string        `json:"publicDescription"`
	PrivateProperties []byte        `state:"hash" json:"privateProperties"`
}

func TestMarshalTagsPointerToStruct(t *testing.T) {
	n := &Namespace{}
	h := &House{
		Address:   "Universe Drive",
		Valuation: 1000,
		LinearID:  "An ID",
		Owner:     []byte("Apple"),
	}
	h2, mapping, err := n.marshalTags(nil, h)
	assert.NoError(t, err)
	err = n.unmarshalTags(nil, h2, mapping)
	assert.NoError(t, err)
	assert.Equal(t, h, h2)

	id, err := n.getStateID(h2)
	assert.NoError(t, err)
	assert.Equal(t, id, h2.(*House).LinearID)
}

func TestMarshalTagsPointerToStruct2(t *testing.T) {
	n := &Namespace{}
	h := &Asset{
		ObjectType:        "otype",
		ID:                "1234",
		PublicDescription: "public",
		PrivateProperties: []byte("private"),
		Owner:             []byte("Apple"),
	}
	h2, mapping, err := n.marshalTags(nil, h)
	assert.NoError(t, err)
	err = n.unmarshalTags(nil, h2, mapping)
	assert.NoError(t, err)
	assert.Equal(t, h, h2)
}
