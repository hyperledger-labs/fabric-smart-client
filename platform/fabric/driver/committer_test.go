/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

func TestStatusName(t *testing.T) {
	t.Parallel()

	require.Equal(t, "valid", driver.StatusName(driver.Valid))
	require.Equal(t, "invalid", driver.StatusName(driver.Invalid))
	require.Equal(t, "busy", driver.StatusName(driver.Busy))
	require.Equal(t, "unknown", driver.StatusName(driver.Unknown))
	require.Equal(t, "unknown(99)", driver.StatusName(driver.ValidationCode(99)))
}

func TestErrFinalityTimeout(t *testing.T) {
	t.Parallel()

	wrapped := errors.Wrap(driver.ErrFinalityTimeout, "timed out")
	require.True(t, errors.Is(wrapped, driver.ErrFinalityTimeout))
	require.ErrorIs(t, wrapped, driver.ErrFinalityTimeout)
}
