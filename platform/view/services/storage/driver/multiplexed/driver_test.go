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
				t.Helper()
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
				t.Helper()
				require.Contains(t, d.drivers, driver.PersistenceType("postgres"))
				require.Contains(t, d.drivers, driver.PersistenceType("sqlite"))
			},
		},
	}

	for _, tt := range tests {
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

	t.Run("success without params", func(t *testing.T) {
		t.Parallel()

		mockDriver := &mock.Driver{}
		mockDriver.NewKVSReturns(&mock.KeyValueStore{}, nil)

		config := &mock.Config{}
		config.UnmarshalKeyStub = func(key string, v any) error {
			if ptr, ok := v.(*driver.PersistenceType); ok {
				*ptr = "postgres"
			}
			return nil
		}

		d := NewDriver(config, driver2.NamedDriver{Name: "postgres", Driver: mockDriver})
		kvs, err := d.NewKVS("test-persistence")

		require.NoError(t, err)
		require.NotNil(t, kvs)
	})

	t.Run("success with params", func(t *testing.T) {
		t.Parallel()

		mockDriver := &mock.Driver{}
		mockDriver.NewKVSReturns(&mock.KeyValueStore{}, nil)

		config := &mock.Config{}
		config.UnmarshalKeyStub = func(key string, v any) error {
			if ptr, ok := v.(*driver.PersistenceType); ok {
				*ptr = "postgres"
			}
			return nil
		}

		d := NewDriver(config, driver2.NamedDriver{Name: "postgres", Driver: mockDriver})
		kvs, err := d.NewKVS("test-persistence", "param1", "param2")

		require.NoError(t, err)
		require.NotNil(t, kvs)
	})

	t.Run("defaults to memory when type is empty", func(t *testing.T) {
		t.Parallel()

		mockDriver := &mock.Driver{}
		mockDriver.NewKVSReturns(&mock.KeyValueStore{}, nil)

		config := &mock.Config{}
		config.UnmarshalKeyStub = func(key string, v any) error {
			if ptr, ok := v.(*driver.PersistenceType); ok {
				*ptr = ""
			}
			return nil
		}

		d := NewDriver(config, driver2.NamedDriver{Name: mem.Persistence, Driver: mockDriver})
		kvs, err := d.NewKVS("test-persistence")

		require.NoError(t, err)
		require.NotNil(t, kvs)
	})

	t.Run("error from config", func(t *testing.T) {
		t.Parallel()

		config := &mock.Config{}
		config.UnmarshalKeyReturns(errors.New("config error"))

		d := NewDriver(config)
		kvs, err := d.NewKVS("test-persistence")

		require.Error(t, err)
		require.Nil(t, kvs)
		require.Contains(t, err.Error(), "config error")
	})

	t.Run("driver not found", func(t *testing.T) {
		t.Parallel()

		config := &mock.Config{}
		config.UnmarshalKeyStub = func(key string, v any) error {
			if ptr, ok := v.(*driver.PersistenceType); ok {
				*ptr = "postgres"
			}
			return nil
		}

		// Intentionally not providing any drivers
		d := NewDriver(config)
		kvs, err := d.NewKVS("test-persistence")

		require.Error(t, err)
		require.Nil(t, kvs)
		require.Contains(t, err.Error(), "driver postgres not found")
	})

	t.Run("error from driver", func(t *testing.T) {
		t.Parallel()

		mockDriver := &mock.Driver{}
		mockDriver.NewKVSReturns(nil, errors.New("driver error"))

		config := &mock.Config{}
		config.UnmarshalKeyStub = func(key string, v any) error {
			if ptr, ok := v.(*driver.PersistenceType); ok {
				*ptr = "postgres"
			}
			return nil
		}

		d := NewDriver(config, driver2.NamedDriver{Name: "postgres", Driver: mockDriver})
		kvs, err := d.NewKVS("test-persistence")

		require.Error(t, err)
		require.Nil(t, kvs)
		require.Contains(t, err.Error(), "driver error")
	})
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

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mockDriver := &mock.Driver{}
		mockDriver.NewBindingReturns(&mock.BindingStore{}, nil)

		d := NewDriver(config, driver2.NamedDriver{Name: "sqlite", Driver: mockDriver})

		bs, err := d.NewBinding("test", "param1")
		require.NoError(t, err)
		require.NotNil(t, bs)
	})

	t.Run("error from driver", func(t *testing.T) {
		t.Parallel()

		mockDriver := &mock.Driver{}
		mockDriver.NewBindingReturns(nil, errors.New("binding error"))

		d := NewDriver(config, driver2.NamedDriver{Name: "sqlite", Driver: mockDriver})

		bs, err := d.NewBinding("test")
		require.Error(t, err)
		require.Nil(t, bs)
	})
}

func TestDriver_NewSignerInfo(t *testing.T) {
	t.Parallel()

	config := &mock.Config{}
	config.UnmarshalKeyStub = func(key string, v any) error {
		if ptr, ok := v.(*driver.PersistenceType); ok {
			*ptr = "postgres"
		}
		return nil
	}

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mockDriver := &mock.Driver{}
		mockDriver.NewSignerInfoReturns(&mock.SignerInfoStore{}, nil)

		d := NewDriver(config, driver2.NamedDriver{Name: "postgres", Driver: mockDriver})

		sis, err := d.NewSignerInfo("test", "param1")
		require.NoError(t, err)
		require.NotNil(t, sis)
	})

	t.Run("error from driver", func(t *testing.T) {
		t.Parallel()

		mockDriver := &mock.Driver{}
		mockDriver.NewSignerInfoReturns(nil, errors.New("signer error"))

		d := NewDriver(config, driver2.NamedDriver{Name: "postgres", Driver: mockDriver})

		sis, err := d.NewSignerInfo("test")
		require.Error(t, err)
		require.Nil(t, sis)
	})
}

func TestDriver_NewAuditInfo(t *testing.T) {
	t.Parallel()

	config := &mock.Config{}
	config.UnmarshalKeyStub = func(key string, v any) error {
		if ptr, ok := v.(*driver.PersistenceType); ok {
			*ptr = "memory"
		}
		return nil
	}

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mockDriver := &mock.Driver{}
		mockDriver.NewAuditInfoReturns(&mock.AuditInfoStore{}, nil)

		d := NewDriver(config, driver2.NamedDriver{Name: "memory", Driver: mockDriver})

		ais, err := d.NewAuditInfo("test", "param1", "param2")
		require.NoError(t, err)
		require.NotNil(t, ais)
	})

	t.Run("error from driver", func(t *testing.T) {
		t.Parallel()

		mockDriver := &mock.Driver{}
		mockDriver.NewAuditInfoReturns(nil, errors.New("audit error"))

		d := NewDriver(config, driver2.NamedDriver{Name: "memory", Driver: mockDriver})

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
