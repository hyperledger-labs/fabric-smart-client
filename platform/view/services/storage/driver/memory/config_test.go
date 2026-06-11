/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mem

import (
	"testing"

	"github.com/stretchr/testify/require"

	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
)

func TestOptsProvider_GetOpts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		params []string
	}{
		{name: "no parameters", params: nil},
		{name: "with parameters", params: []string{"param1", "param2"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			provider := &optsProvider{}
			opts := provider.GetOpts(tc.params...)

			require.Equal(t, "file::memory:?cache=shared", opts.DataSource)
			require.False(t, opts.SkipPragmas)
			require.Equal(t, 10, opts.MaxOpenConns)
			require.Equal(t, common2.DefaultMaxIdleConns, opts.MaxIdleConns)
			require.Equal(t, common2.DefaultMaxIdleTime, opts.MaxIdleTime)
			require.Empty(t, opts.TablePrefix)
			require.Equal(t, tc.params, opts.TableNameParams)
			require.Nil(t, opts.Tracing)
		})
	}
}

func TestOptsProvider_GetConfig(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		params []string
	}{
		{name: "no parameters", params: nil},
		{name: "with parameters", params: []string{"param1", "param2"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			provider := &optsProvider{}
			cfg := provider.GetConfig(tc.params...)

			require.Equal(t, "file::memory:?cache=shared", cfg.DataSource)
			require.False(t, cfg.SkipPragmas)
			require.Equal(t, 10, cfg.MaxOpenConns)
			require.NotNil(t, cfg.MaxIdleConns)
			require.Equal(t, common2.DefaultMaxIdleConns, *cfg.MaxIdleConns)
			require.NotNil(t, cfg.MaxIdleTime)
			require.Equal(t, common2.DefaultMaxIdleTime, *cfg.MaxIdleTime)
			require.False(t, cfg.SkipCreateTable)
			require.Empty(t, cfg.TablePrefix)
			require.Equal(t, tc.params, cfg.TableNameParams)
		})
	}
}

func TestOptsProvider_GetConfig_PointerIndependence(t *testing.T) {
	t.Parallel()

	provider := &optsProvider{}
	cfg1 := provider.GetConfig()
	cfg2 := provider.GetConfig()

	require.NotSame(t, cfg1.MaxIdleConns, cfg2.MaxIdleConns)
	require.NotSame(t, cfg1.MaxIdleTime, cfg2.MaxIdleTime)

	*cfg1.MaxIdleConns = 100
	require.Equal(t, common2.DefaultMaxIdleConns, *cfg2.MaxIdleConns)
}

func TestOp_GlobalVariable(t *testing.T) {
	t.Parallel()

	require.NotNil(t, Op)
	require.IsType(t, &optsProvider{}, Op)

	opts := Op.GetOpts("test")
	require.Equal(t, "file::memory:?cache=shared", opts.DataSource)

	cfg := Op.GetConfig("test")
	require.Equal(t, "file::memory:?cache=shared", cfg.DataSource)
}
