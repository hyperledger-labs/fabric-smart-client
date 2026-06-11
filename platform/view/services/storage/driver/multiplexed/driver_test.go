/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package multiplexed

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/multiplexed/mock"
)

//go:generate counterfeiter -o mock/driver.go --fake-name Driver github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver.Driver
//go:generate counterfeiter -o mock/config.go --fake-name Config github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver.Config
//go:generate counterfeiter -o mock/kvs.go --fake-name KeyValueStore github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver.KeyValueStore
//go:generate counterfeiter -o mock/binding_store.go --fake-name BindingStore github.com/hyperledger-labs/fabric-smart-client/platform/common/driver.BindingStore
//go:generate counterfeiter -o mock/signer_info_store.go --fake-name SignerInfoStore github.com/hyperledger-labs/fabric-smart-client/platform/common/driver.SignerInfoStore
//go:generate counterfeiter -o mock/audit_info_store.go --fake-name AuditInfoStore github.com/hyperledger-labs/fabric-smart-client/platform/common/driver.AuditInfoStore

func TestNewDriver(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		namedDrivers  []driver2.NamedDriver
		expectedCount int
		checkDrivers  func(*testing.T, Driver)
	}{
		{
			name:          "no named drivers",
			namedDrivers:  nil,
			expectedCount: 0,
		},
		{
			name: "single named driver",
			namedDrivers: []driver2.NamedDriver{
				{Name: "postgres", Driver: &mock.Driver{}},
			},
			expectedCount: 1,
			checkDrivers: func(t *testing.T, d Driver) {
				require.Contains(t, d.drivers, driver.PersistenceType("postgres"))
			},
		},
		{
			name: "multiple named drivers",
			namedDrivers: []driver2.NamedDriver{
				{Name: "postgres", Driver: &mock.Driver{}},
				{Name: "sqlite", Driver: &mock.Driver{}},
			},
			expectedCount: 2,
			checkDrivers: func(t *testing.T, d Driver) {
				require.Contains(t, d.drivers, driver.PersistenceType("postgres"))
				require.Contains(t, d.drivers, driver.PersistenceType("sqlite"))
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			config := &mock.Config{}
			d := NewDriver(config, tt.namedDrivers...)

			require.NotNil(t, d)
			require.NotNil(t, d.drivers)
			require.Len(t, d.drivers, tt.expectedCount)
			require.NotNil(t, d.config)
			if tt.checkDrivers != nil {
				tt.checkDrivers(t, d)
			}
		})
	}
}

func TestDriver_NewKVS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		driverType  driver.PersistenceType
		configErr   error
		driverErr   error
		params      []string
		expectErr   bool
		errContains string
	}{
		{
			name:       "success without params",
			driverType: "postgres",
			params:     nil,
		},
		{
			name:       "success with params",
			driverType: "postgres",
			params:     []string{"param1", "param2"},
		},
		{
			name:       "defaults to memory when type is empty",
			driverType: "",
		},
		{
			name:        "error from config",
			configErr:   errors.New("config error"),
			expectErr:   true,
			errContains: "config error",
		},
		{
			name:        "driver not found",
			driverType:  "postgres",
			expectErr:   true,
			errContains: "driver postgres not found",
		},
		{
			name:        "error from driver",
			driverType:  "postgres",
			driverErr:   errors.New("driver error"),
			expectErr:   true,
			errContains: "driver error",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			config := &mock.Config{}
			if tt.configErr != nil {
				config.UnmarshalKeyReturns(tt.configErr)
			} else {
				config.UnmarshalKeyStub = func(key string, v any) error {
					if ptr, ok := v.(*driver.PersistenceType); ok {
						*ptr = tt.driverType
					}
					return nil
				}
			}

			var namedDrivers []driver2.NamedDriver
			if tt.name != "driver not found" && tt.configErr == nil {
				mockDriver := &mock.Driver{}
				if tt.driverErr != nil {
					mockDriver.NewKVSReturns(nil, tt.driverErr)
				} else {
					mockDriver.NewKVSReturns(&mock.KeyValueStore{}, nil)
				}
				driverName := tt.driverType
				if driverName == "" {
					driverName = mem.Persistence
				}
				namedDrivers = []driver2.NamedDriver{{Name: driverName, Driver: mockDriver}}
			}

			d := NewDriver(config, namedDrivers...)
			kvs, err := d.NewKVS("test-persistence", tt.params...)

			if tt.expectErr {
				require.Error(t, err)
				require.Nil(t, kvs)
				require.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
				require.NotNil(t, kvs)
			}
		})
	}
}

func TestDriver_NewBinding(t *testing.T) {
	t.Parallel()

	mockDriver := &mock.Driver{}
	mockDriver.NewBindingReturns(&mock.BindingStore{}, nil)

	config := &mock.Config{}
	config.UnmarshalKeyStub = func(key string, v any) error {
		if ptr, ok := v.(*driver.PersistenceType); ok {
			*ptr = "sqlite"
		}
		return nil
	}

	d := NewDriver(config, driver2.NamedDriver{Name: "sqlite", Driver: mockDriver})

	t.Run("success", func(t *testing.T) {
		bs, err := d.NewBinding("test", "param1")
		require.NoError(t, err)
		require.NotNil(t, bs)
	})

	t.Run("error from driver", func(t *testing.T) {
		mockDriver.NewBindingReturns(nil, errors.New("binding error"))
		bs, err := d.NewBinding("test")
		require.Error(t, err)
		require.Nil(t, bs)
	})
}

func TestDriver_NewSignerInfo(t *testing.T) {
	t.Parallel()

	mockDriver := &mock.Driver{}
	mockDriver.NewSignerInfoReturns(&mock.SignerInfoStore{}, nil)

	config := &mock.Config{}
	config.UnmarshalKeyStub = func(key string, v any) error {
		if ptr, ok := v.(*driver.PersistenceType); ok {
			*ptr = "postgres"
		}
		return nil
	}

	d := NewDriver(config, driver2.NamedDriver{Name: "postgres", Driver: mockDriver})

	t.Run("success", func(t *testing.T) {
		sis, err := d.NewSignerInfo("test", "param1")
		require.NoError(t, err)
		require.NotNil(t, sis)
	})

	t.Run("error from driver", func(t *testing.T) {
		mockDriver.NewSignerInfoReturns(nil, errors.New("signer error"))
		sis, err := d.NewSignerInfo("test")
		require.Error(t, err)
		require.Nil(t, sis)
	})
}

func TestDriver_NewAuditInfo(t *testing.T) {
	t.Parallel()

	mockDriver := &mock.Driver{}
	mockDriver.NewAuditInfoReturns(&mock.AuditInfoStore{}, nil)

	config := &mock.Config{}
	config.UnmarshalKeyStub = func(key string, v any) error {
		if ptr, ok := v.(*driver.PersistenceType); ok {
			*ptr = "memory"
		}
		return nil
	}

	d := NewDriver(config, driver2.NamedDriver{Name: "memory", Driver: mockDriver})

	t.Run("success", func(t *testing.T) {
		ais, err := d.NewAuditInfo("test", "param1", "param2")
		require.NoError(t, err)
		require.NotNil(t, ais)
	})

	t.Run("error from driver", func(t *testing.T) {
		mockDriver.NewAuditInfoReturns(nil, errors.New("audit error"))
		ais, err := d.NewAuditInfo("test")
		require.Error(t, err)
		require.Nil(t, ais)
	})
}

func TestDriver_getDriver(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		driverType  driver.PersistenceType
		configErr   error
		registered  bool
		expectErr   bool
		errContains string
	}{
		{
			name:       "returns configured driver",
			driverType: "postgres",
			registered: true,
		},
		{
			name:       "defaults to memory when empty",
			driverType: "",
			registered: true,
		},
		{
			name:        "error from config",
			configErr:   errors.New("config error"),
			expectErr:   true,
			errContains: "config error",
		},
		{
			name:        "driver not registered",
			driverType:  "unregistered",
			registered:  false,
			expectErr:   true,
			errContains: "driver unregistered not found",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			config := &mock.Config{}
			if tt.configErr != nil {
				config.UnmarshalKeyReturns(tt.configErr)
			} else {
				config.UnmarshalKeyStub = func(key string, v any) error {
					if ptr, ok := v.(*driver.PersistenceType); ok {
						*ptr = tt.driverType
					}
					return nil
				}
			}

			var namedDrivers []driver2.NamedDriver
			if tt.registered {
				mockDriver := &mock.Driver{}
				driverName := tt.driverType
				if driverName == "" {
					driverName = mem.Persistence
				}
				namedDrivers = []driver2.NamedDriver{{Name: driverName, Driver: mockDriver}}
			}

			d := NewDriver(config, namedDrivers...)
			dr, err := d.getDriver("test-persistence")

			if tt.expectErr {
				require.Error(t, err)
				require.Nil(t, dr)
				require.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
				require.NotNil(t, dr)
			}
		})
	}
}