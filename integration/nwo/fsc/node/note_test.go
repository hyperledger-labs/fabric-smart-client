/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

type DummySDK struct {
}

func NewDummySDK() *DummySDK {
	return &DummySDK{}
}

func (d *DummySDK) Install() error {
	panic("implement me")
}

func (d *DummySDK) Start(ctx context.Context) error {
	panic("implement me")
}

func TestNode_AddSDKWithBase(t *testing.T) {
	node := NewNode("pineapple")
	n := node.AddSDKWithBase(&DummySDK{}, &DummySDK{}, &DummySDK{})
	assert.NotNil(t, n)
	assert.Equal(t, "node.NewFrom(node.NewFrom(node.NewDummySDK(n)))", n.SDKs[0].Type)
}
