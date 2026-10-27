/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	fscconfig "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/mock"
	drivertesting "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/testing"
)

// TestGetOptsAppliesDefaults checks unset idle settings are defaulted and the table-name
// params and tracing config are filled in.
func TestGetOptsAppliesDefaults(t *testing.T) {
	t.Parallel()

	cp := NewConfigProvider(drivertesting.MockConfig(Config{DataSource: "file:test"}))

	o, err := cp.GetOpts("", "p1", "p2")
	require.NoError(t, err)

	require.NotNil(t, o.MaxIdleConns)
	assert.Equal(t, common.DefaultMaxIdleConns, *o.MaxIdleConns)
	require.NotNil(t, o.MaxIdleTime)
	assert.Equal(t, common.DefaultMaxIdleTime, *o.MaxIdleTime)
	assert.Equal(t, []string{"p1", "p2"}, o.TableNameParams)
	assert.NotNil(t, o.Tracing)
}

// TestGetOptsKeepsExplicitIdleSettings checks configured idle settings are not overwritten.
func TestGetOptsKeepsExplicitIdleSettings(t *testing.T) {
	t.Parallel()

	conns := 7
	idle := 3 * time.Second
	cp := NewConfigProvider(drivertesting.MockConfig(Config{DataSource: "file:test", MaxIdleConns: &conns, MaxIdleTime: &idle}))

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

	cp := NewConfigProvider(drivertesting.MockConfig(Config{}))

	_, err := cp.GetOpts("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing data source")
}

// TestGetOptsUnmarshalError checks a config that fails to unmarshal is reported.
func TestGetOptsUnmarshalError(t *testing.T) {
	t.Parallel()

	raw := &mock.ConfigProvider{}
	raw.UnmarshalKeyReturns(errors.New("bad config"))
	cp := NewConfigProvider(common.NewConfig(raw))

	_, err := cp.GetOpts("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad config")
}

// TestGetOptsFromEnv checks that CORE_FSC_PERSISTENCES_<NAME>_OPTS_DATASOURCE overrides the
// data source of a persistence, and that a variable under the store key that names the
// persistence replaces the name, so the store can no longer resolve its persistence.
func TestGetOptsFromEnv(t *testing.T) { //nolint:paralleltest // t.Setenv
	t.Setenv("CORE_FSC_PERSISTENCES_DEFAULT_OPTS_DATASOURCE", "from-env")
	t.Setenv("CORE_FSC_KVS_PERSISTENCE_OPTS_DATASOURCE", "replaces-the-name")
	dir := t.TempDir()
	yaml := `
fsc:
  persistences:
    default:
      type: sqlite
      opts:
        dataSource: from-file
  kvs:
    persistence: default
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "core.yaml"), []byte(yaml), 0o600))
	p, err := fscconfig.NewProvider(dir)
	require.NoError(t, err)

	o, err := NewConfigProvider(common.NewConfig(p)).GetOpts("default")
	require.NoError(t, err)
	assert.Equal(t, "from-env", o.DataSource)

	_, err = common.GetPersistenceName(p, "fsc.kvs.persistence")
	require.ErrorContains(t, err, "unconvertible type")
}
