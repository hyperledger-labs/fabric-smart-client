/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/mock"
	testing2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/testing"
)

// TestGetOptsAppliesDefaults checks unset idle settings are defaulted and the table-name
// params and tracing config are filled in.
func TestGetOptsAppliesDefaults(t *testing.T) {
	t.Parallel()

	cp := NewConfigProvider(testing2.MockConfig(Config{DataSource: "file:test"}))

	o, err := cp.GetOpts("", "p1", "p2")
	require.NoError(t, err)

	require.NotNil(t, o.MaxIdleConns)
	assert.Equal(t, common3.DefaultMaxIdleConns, *o.MaxIdleConns)
	require.NotNil(t, o.MaxIdleTime)
	assert.Equal(t, common3.DefaultMaxIdleTime, *o.MaxIdleTime)
	assert.Equal(t, []string{"p1", "p2"}, o.TableNameParams)
	assert.NotNil(t, o.Tracing)
}

// TestGetOptsKeepsExplicitIdleSettings checks configured idle settings are not overwritten.
func TestGetOptsKeepsExplicitIdleSettings(t *testing.T) {
	t.Parallel()

	conns := 7
	idle := 3 * time.Second
	cp := NewConfigProvider(testing2.MockConfig(Config{DataSource: "file:test", MaxIdleConns: &conns, MaxIdleTime: &idle}))

	o, err := cp.GetOpts("")
	require.NoError(t, err)
	require.NotNil(t, o.MaxIdleConns)
	require.NotNil(t, o.MaxIdleTime)
	assert.Equal(t, 7, *o.MaxIdleConns)
	assert.Equal(t, 3*time.Second, *o.MaxIdleTime)
}

// TestGetOptsMissingDataSource checks an empty data source is rejected.
func TestGetOptsMissingDataSource(t *testing.T) {
	t.Parallel()

	cp := NewConfigProvider(testing2.MockConfig(Config{}))

	_, err := cp.GetOpts("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing data source")
}

// TestGetOptsUnmarshalError checks a config that fails to unmarshal is reported.
func TestGetOptsUnmarshalError(t *testing.T) {
	t.Parallel()

	raw := &mock.ConfigProvider{}
	raw.UnmarshalKeyReturns(errors.New("bad config"))
	cp := NewConfigProvider(common3.NewConfig(raw))

	_, err := cp.GetOpts("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad config")
}
