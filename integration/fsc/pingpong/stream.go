/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pingpong

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/assert"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type StreamerView struct{}

func (s *StreamerView) Call(context view.Context) (interface{}, error) {
	stream := view2.GetStream(context)
	assert.NoError(stream.Send("hello"), "failed to send hello")
	var msg string
	assert.NoError(stream.Recv(&msg), "failed to receive message")
	assert.Equal("ciao", msg)

	return "OK", nil
}

// StreamerViewFactory is the factory of Initiator views
type StreamerViewFactory struct{}

// NewView returns a new instance of the Initiator view
func (i *StreamerViewFactory) NewView(in []byte) (view.View, error) {
	return &StreamerView{}, nil
}
