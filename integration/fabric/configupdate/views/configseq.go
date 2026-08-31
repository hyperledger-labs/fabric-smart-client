/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// ConfigSequenceView reports the sequence number of the channel configuration
// the FSC node running it currently holds.
//
// This is the suite's window onto the committer's CONFIG-block path: the
// sequence advances only if a configuration block reached the node and was
// applied, so a node that silently drops one keeps reporting the sequence it
// started from.
type ConfigSequenceView struct{}

func (v *ConfigSequenceView) Call(viewCtx view.Context) (any, error) {
	_, ch, err := fabric.GetDefaultChannel(viewCtx)
	assert.NoError(err, "failed getting the default channel")

	sequence, err := ch.ConfigSequence()
	assert.NoError(err, "failed getting the channel configuration sequence")

	return sequence, nil
}

type ConfigSequenceViewFactory struct{}

func (f *ConfigSequenceViewFactory) NewView([]byte) (view.View, error) {
	return &ConfigSequenceView{}, nil
}
