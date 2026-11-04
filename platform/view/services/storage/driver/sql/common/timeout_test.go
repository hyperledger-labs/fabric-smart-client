/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
)

func TestQueryTimeoutContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := common.QueryTimeout(0).Context(t.Context())
	cancel()
	_, ok := ctx.Deadline()
	assert.False(t, ok, "zero adds no deadline")

	ctx, cancel = common.QueryTimeout(time.Minute).Context(t.Context())
	defer cancel()
	deadline, ok := ctx.Deadline()
	require.True(t, ok)
	assert.WithinDuration(t, time.Now().Add(time.Minute), deadline, 5*time.Second)
	cancel()
	assert.ErrorIs(t, ctx.Err(), context.Canceled)
}

func TestQueryTimeoutValidate(t *testing.T) {
	t.Parallel()

	for _, ok := range []time.Duration{0, 100 * time.Millisecond, 30 * time.Second} {
		require.NoError(t, common.QueryTimeout(ok).Validate(), ok)
	}
	// 30 is what a value written without a unit decodes to.
	for _, bad := range []time.Duration{-time.Second, 30, 99 * time.Millisecond} {
		assert.ErrorContains(t, common.QueryTimeout(bad).Validate(), "queryTimeout", bad)
	}
}
