/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
)

// TestIsolationLevelsMap checks the defined levels map to their sql equivalents and an
// undefined level is rejected.
func TestIsolationLevelsMap(t *testing.T) {
	t.Parallel()

	il, err := IsolationLevels.Map(cdriver.LevelDefault)
	require.NoError(t, err)
	assert.Equal(t, sql.LevelDefault, il)

	il, err = IsolationLevels.Map(cdriver.LevelReadCommitted)
	require.NoError(t, err)
	assert.Equal(t, sql.LevelReadCommitted, il)

	_, err = IsolationLevels.Map(cdriver.IsolationLevel(99))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "99", "the requested level is reported")
}
