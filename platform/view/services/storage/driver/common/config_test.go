/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"testing"

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
