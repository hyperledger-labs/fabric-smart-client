/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package queryservice_test

import (
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/vault/queryservice"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

type configService struct {
	V *viper.Viper
}

func newConfigService(c map[string]any) *configService {
	v := viper.New()
	for k, val := range c {
		v.Set(k, val)
	}
	return &configService{V: v}
}

func (c configService) UnmarshalKey(key string, rawVal interface{}) error {
	return c.V.UnmarshalKey(key, rawVal)
}

func TestConfig(t *testing.T) {
	table := []struct {
		name   string
		cfg    map[string]any
		checks func(t *testing.T, config *queryservice.Config)
	}{
		{
			name: "empty config",
			cfg:  nil,
			checks: func(t *testing.T, c *queryservice.Config) {
				t.Helper()
				require.Equal(t, queryservice.DefaultQueryTimeout, c.QueryTimeout)
				require.Empty(t, c.Endpoints)
			},
		},
		{
			name: "override default",
			cfg:  map[string]any{"queryService.queryTimeout": 10 * time.Second},
			checks: func(t *testing.T, c *queryservice.Config) {
				t.Helper()
				require.Equal(t, 10*time.Second, c.QueryTimeout)
				require.Empty(t, c.Endpoints)
			},
		},
		{
			name: "single endpoints",
			cfg: map[string]any{
				"queryService.queryTimeout": 10 * time.Second,
				"queryService.Endpoints": []any{
					map[string]any{
						"address":           "localhost:9999",
						"connectionTimeout": 5 * time.Second,
						"tlsEnabled":        false,
						"tlsRootCertFile":   "/tmp/rootcert",
					},
				},
			},
			checks: func(t *testing.T, c *queryservice.Config) {
				t.Helper()
				require.Equal(t, 10*time.Second, c.QueryTimeout)
				require.NotEmpty(t, c.Endpoints)
				require.Equal(t, "localhost:9999", c.Endpoints[0].Address)
				require.Equal(t, 5*time.Second, c.Endpoints[0].ConnectionTimeout)
				require.False(t, c.Endpoints[0].TLSEnabled)
				require.Equal(t, "/tmp/rootcert", c.Endpoints[0].TLSRootCertFile)
			},
		},
	}

	for _, tc := range table {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cs := newConfigService(tc.cfg)
			c, err := queryservice.NewConfig(cs)
			require.NoError(t, err)
			tc.checks(t, c)
		})
	}
}
