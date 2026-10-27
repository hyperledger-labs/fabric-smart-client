/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

func TestConfig_GetDriverType(t *testing.T) {
	t.Parallel()

	const withDefault = `
fsc:
  persistences:
    default:
      type: memory
    mydb:
      type: sqlite
    notype:
      typ: sqlite
    emptytype:
      type: ""
    scalar: sqlite
    nested:
      type:
        inner: x
`
	const withoutDefault = `
fsc:
  persistences:
    mydb:
      type: sqlite
`
	const defaultWithoutType = `
fsc:
  persistences:
    default:
      opts: {}
`

	tests := []struct {
		name        string
		yaml        string
		persistence dbdriver.PersistenceName
		want        driver.PersistenceType
		errContains string
	}{
		{name: "named block", yaml: withDefault, persistence: "mydb", want: "sqlite"},
		{name: "empty name resolves to default block", yaml: withDefault, persistence: "", want: "memory"},
		{name: "explicit default name", yaml: withDefault, persistence: "default", want: "memory"},
		{name: "empty name without default block", yaml: withoutDefault, persistence: "", want: ""},
		{name: "unknown name", yaml: withDefault, persistence: "mydbb", errContains: "persistence [mydbb] is not defined under fsc.persistences"},
		{name: "explicit default name without default block", yaml: withoutDefault, persistence: "default", errContains: "persistence [default] is not defined"},
		{name: "block without type", yaml: withDefault, persistence: "notype", errContains: "fsc.persistences.notype.type is not set"},
		{name: "block with empty type", yaml: withDefault, persistence: "emptytype", errContains: "fsc.persistences.emptytype.type is not set"},
		{name: "scalar block without type", yaml: withDefault, persistence: "scalar", errContains: "fsc.persistences.scalar.type is not set"},
		{name: "type is not a string", yaml: withDefault, persistence: "nested", errContains: "failed to unmarshal fsc.persistences.nested.type"},
		{name: "default block without type", yaml: defaultWithoutType, persistence: "", errContains: "fsc.persistences.default.type is not set"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cp, err := (&config.Provider{}).ProvideFromRaw([]byte(tt.yaml))
			require.NoError(t, err)

			got, err := NewConfig(cp).GetDriverType(tt.persistence)
			if tt.errContains != "" {
				require.ErrorContains(t, err, tt.errContains)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestGetPersistenceName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		want    dbdriver.PersistenceName
		wantErr string
	}{
		{name: "unset", value: "{}"},
		{name: "name", value: "{persistence: token_persistence}", want: "token_persistence"},
		{name: "map", value: "{persistence: {opts: {dataSource: host=db password=SECRET}}}", wantErr: "failed to unmarshal"},
		{name: "dsn from file", value: "{persistence: {file: " + writeFile(t, "host=db password=SECRET\n") + "}}", wantErr: "is not a valid persistence name"},
		{name: "dsn", value: "{persistence: 'host=db password=SECRET'}", wantErr: "is not a valid persistence name"},
		{name: "url dsn", value: "{persistence: 'postgres://u:SECRET@db/x'}", wantErr: "is not a valid persistence name"},
		{name: "dotted", value: "{persistence: a.b}", wantErr: "is not a valid persistence name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cp, err := (&config.Provider{}).ProvideFromRaw([]byte("store: " + tt.value))
			require.NoError(t, err)

			got, err := GetPersistenceName(cp, "store.persistence")
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.ErrorContains(t, err, "store.persistence")
				assert.NotContains(t, err.Error(), "SECRET")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// writeFile writes content to a new file in a test temp dir and returns its path.
func writeFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "f")
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	return p
}
