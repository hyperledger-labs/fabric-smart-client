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
)

func TestCommitterFLM(t *testing.T) {
	t.Parallel()

	mockCommitter := &mockCommitter{}
	mockChannel := &mockChannel{}
	mockChannel.On("Committer").Return(mockCommitter)

	committer := fabric.NewCommitter(mockChannel)
	flm := NewCommitterFLM(committer)

	t.Run("AddFinalityListener", func(t *testing.T) {
		t.Parallel()
		mockCommitter.On("AddFinalityListener", "tx1", mock.Anything).Return(nil).Once()
		err := flm.AddFinalityListener("", "tx1", &mockFinalityListener{})
		assert.NoError(t, err)
	})

	t.Run("RemoveFinalityListener", func(t *testing.T) {
		t.Parallel()
		mockCommitter.On("RemoveFinalityListener", "tx1", mock.Anything).Return(nil).Once()
		err := flm.RemoveFinalityListener("tx1", &mockFinalityListener{})
		assert.NoError(t, err)
	})
}
