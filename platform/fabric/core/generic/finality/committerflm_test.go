/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/finality/fake"
)

func TestCommitterFLM(t *testing.T) {
	t.Parallel()

	setup := func() (*fake.Committer, *committerListenerManager) {
		mockCommitter := &fake.Committer{}
		mockChannel := &fake.Channel{}
		mockChannel.On("Committer").Return(mockCommitter)

		committer := fabric.NewCommitter(mockChannel)
		flm := NewCommitterFLM(committer)
		return mockCommitter, flm
	}

	t.Run("AddFinalityListener", func(t *testing.T) {
		t.Parallel()
		mockCommitter, flm := setup()
		mockCommitter.On("AddFinalityListener", "tx1", mock.Anything).Return(nil).Once()
		err := flm.AddFinalityListener("", "tx1", &fake.FinalityListener{})
		assert.NoError(t, err)
	})

	t.Run("RemoveFinalityListener", func(t *testing.T) {
		t.Parallel()
		mockCommitter, flm := setup()
		mockCommitter.On("RemoveFinalityListener", "tx1", mock.Anything).Return(nil).Once()
		err := flm.RemoveFinalityListener("tx1", &fake.FinalityListener{})
		assert.NoError(t, err)
	})
}
