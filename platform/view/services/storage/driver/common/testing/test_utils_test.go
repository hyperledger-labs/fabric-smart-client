/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package testing

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
)

// TestConfig is a sample config struct for testing
type TestConfig struct {
	DataSource   string
	MaxOpenConns int
	Timeout      int
}

func TestMockConfig(t *testing.T) {
	t.Parallel()

	t.Run("creates config with simple struct", func(t *testing.T) {
		t.Parallel()
		expectedConfig := TestConfig{
			DataSource:   "test-datasource",
			MaxOpenConns: 10,
			Timeout:      30,
		}

		config := MockConfig(expectedConfig)

		require.NotNil(t, config)

		// Verify the config can unmarshal the expected values
		var actualConfig TestConfig
		err := config.UnmarshalDriverOpts("test", &actualConfig)
		require.NoError(t, err)
		require.Equal(t, expectedConfig, actualConfig)
	})

	t.Run("creates config with string value", func(t *testing.T) {
		t.Parallel()
		expectedValue := "simple-string-value"

		config := MockConfig(expectedValue)

		require.NotNil(t, config)

		var actualValue string
		err := config.UnmarshalDriverOpts("test", &actualValue)
		require.NoError(t, err)
		require.Equal(t, expectedValue, actualValue)
	})

	t.Run("creates config with int value", func(t *testing.T) {
		t.Parallel()
		expectedValue := 42

		config := MockConfig(expectedValue)

		require.NotNil(t, config)

		var actualValue int
		err := config.UnmarshalDriverOpts("test", &actualValue)
		require.NoError(t, err)
		require.Equal(t, expectedValue, actualValue)
	})

	t.Run("creates config with map value", func(t *testing.T) {
		t.Parallel()
		expectedValue := map[string]interface{}{
			"key1": "value1",
			"key2": 123,
		}

		config := MockConfig(expectedValue)

		require.NotNil(t, config)

		var actualValue map[string]interface{}
		err := config.UnmarshalDriverOpts("test", &actualValue)
		require.NoError(t, err)
		require.Equal(t, expectedValue, actualValue)
	})

	t.Run("creates config with slice value", func(t *testing.T) {
		t.Parallel()
		expectedValue := []string{"item1", "item2", "item3"}

		config := MockConfig(expectedValue)

		require.NotNil(t, config)

		var actualValue []string
		err := config.UnmarshalDriverOpts("test", &actualValue)
		require.NoError(t, err)
		require.Equal(t, expectedValue, actualValue)
	})

	t.Run("creates config with nested struct", func(t *testing.T) {
		t.Parallel()
		type NestedConfig struct {
			Inner TestConfig
			Name  string
		}
		expectedConfig := NestedConfig{
			Inner: TestConfig{
				DataSource:   "nested-datasource",
				MaxOpenConns: 5,
				Timeout:      15,
			},
			Name: "nested-test",
		}

		config := MockConfig(expectedConfig)

		require.NotNil(t, config)

		var actualConfig NestedConfig
		err := config.UnmarshalDriverOpts("test", &actualConfig)
		require.NoError(t, err)
		require.Equal(t, expectedConfig, actualConfig)
	})

	t.Run("creates config with pointer struct", func(t *testing.T) {
		t.Parallel()
		expectedConfig := &TestConfig{
			DataSource:   "pointer-datasource",
			MaxOpenConns: 20,
			Timeout:      60,
		}

		config := MockConfig(expectedConfig)

		require.NotNil(t, config)

		var actualConfig *TestConfig
		err := config.UnmarshalDriverOpts("test", &actualConfig)
		require.NoError(t, err)
		require.Equal(t, expectedConfig, actualConfig)
	})

	t.Run("creates config with zero values", func(t *testing.T) {
		t.Parallel()
		expectedConfig := TestConfig{} // All zero values

		config := MockConfig(expectedConfig)

		require.NotNil(t, config)

		var actualConfig TestConfig
		err := config.UnmarshalDriverOpts("test", &actualConfig)
		require.NoError(t, err)
		require.Equal(t, expectedConfig, actualConfig)
	})

	t.Run("multiple unmarshal calls return same value", func(t *testing.T) {
		t.Parallel()
		expectedConfig := TestConfig{
			DataSource:   "consistent-datasource",
			MaxOpenConns: 15,
			Timeout:      45,
		}

		config := MockConfig(expectedConfig)

		// First unmarshal
		var actualConfig1 TestConfig
		err := config.UnmarshalDriverOpts("test", &actualConfig1)
		require.NoError(t, err)
		require.Equal(t, expectedConfig, actualConfig1)

		// Second unmarshal should return the same value
		var actualConfig2 TestConfig
		err = config.UnmarshalDriverOpts("test", &actualConfig2)
		require.NoError(t, err)
		require.Equal(t, expectedConfig, actualConfig2)
		require.Equal(t, actualConfig1, actualConfig2)
	})

	t.Run("different keys return same value", func(t *testing.T) {
		t.Parallel()
		expectedConfig := TestConfig{
			DataSource:   "key-independent-datasource",
			MaxOpenConns: 25,
			Timeout:      90,
		}

		config := MockConfig(expectedConfig)

		// Unmarshal with different keys should return the same value
		var actualConfig1 TestConfig
		err := config.UnmarshalDriverOpts("key1", &actualConfig1)
		require.NoError(t, err)
		require.Equal(t, expectedConfig, actualConfig1)

		var actualConfig2 TestConfig
		err = config.UnmarshalDriverOpts("key2", &actualConfig2)
		require.NoError(t, err)
		require.Equal(t, expectedConfig, actualConfig2)
	})

	t.Run("returns common.Config type", func(t *testing.T) {
		t.Parallel()
		config := MockConfig(TestConfig{})

		require.NotNil(t, config)
		require.IsType(t, &common.Config{}, config)
	})

	t.Run("handles empty string", func(t *testing.T) {
		t.Parallel()
		expectedValue := ""

		config := MockConfig(expectedValue)

		require.NotNil(t, config)

		var actualValue string
		err := config.UnmarshalDriverOpts("test", &actualValue)
		require.NoError(t, err)
		require.Equal(t, expectedValue, actualValue)
	})

	t.Run("handles nil pointer", func(t *testing.T) {
		t.Parallel()
		var expectedConfig *TestConfig = nil

		config := MockConfig(expectedConfig)

		require.NotNil(t, config)

		var actualConfig *TestConfig
		err := config.UnmarshalDriverOpts("test", &actualConfig)
		require.NoError(t, err)
		require.Equal(t, expectedConfig, actualConfig)
	})
}

func TestMockConfig_TypeSafety(t *testing.T) {
	t.Parallel()

	t.Run("type mismatch causes runtime panic", func(t *testing.T) {
		t.Parallel()
		// Create config with TestConfig
		config := MockConfig(TestConfig{
			DataSource: "type-test",
		})

		// Try to unmarshal into wrong type - this will panic due to type assertion
		var wrongType string
		require.Panics(t, func() {
			_ = config.UnmarshalDriverOpts("test", &wrongType)
		}, "MockConfig should panic when unmarshaling into wrong type")
	})

	t.Run("correct type succeeds", func(t *testing.T) {
		t.Parallel()
		expectedConfig := TestConfig{
			DataSource: "correct-type",
		}
		config := MockConfig(expectedConfig)

		var actualConfig TestConfig
		err := config.UnmarshalDriverOpts("test", &actualConfig)
		require.NoError(t, err)
		require.Equal(t, expectedConfig, actualConfig)
	})

	t.Run("pointer type mismatch causes panic", func(t *testing.T) {
		t.Parallel()
		// Create config with pointer type
		config := MockConfig(&TestConfig{
			DataSource: "pointer-type-test",
		})

		// Try to unmarshal into non-pointer type - this will panic
		var wrongType TestConfig
		require.Panics(t, func() {
			_ = config.UnmarshalDriverOpts("test", &wrongType)
		}, "MockConfig should panic when pointer/non-pointer types mismatch")
	})
}

func TestMockConfig_Limitations(t *testing.T) {
	t.Parallel()

	t.Run("only works with UnmarshalDriverOpts for intended type", func(t *testing.T) {
		t.Parallel()
		// MockConfig is designed for simple use cases where you mock a single config type
		// It doesn't handle complex scenarios like GetDriverType which expects different types
		config := MockConfig(TestConfig{
			DataSource: "limitation-test",
		})

		require.NotNil(t, config)

		// This demonstrates the limitation: MockConfig always returns the same type
		// regardless of the key, so methods like GetDriverType that expect different
		// types will panic. This is acceptable because MockConfig is meant for simple
		// test scenarios where you only need to mock driver options.
		require.Panics(t, func() {
			_, _ = config.GetDriverType("test")
		}, "MockConfig has limitations - it only works for the mocked type")
	})
}

// BenchmarkMockConfig measures the performance of MockConfig
func BenchmarkMockConfig(b *testing.B) {
	config := TestConfig{
		DataSource:   "benchmark-datasource",
		MaxOpenConns: 100,
		Timeout:      120,
	}

	b.Run("creation", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = MockConfig(config)
		}
	})

	b.Run("unmarshal", func(b *testing.B) {
		mockConfig := MockConfig(config)
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			var result TestConfig
			_ = mockConfig.UnmarshalDriverOpts("test", &result)
		}
	})
}

// Example demonstrates basic usage of MockConfig
func ExampleMockConfig() {
	// Create a mock config with test data
	config := MockConfig(TestConfig{
		DataSource:   "example-datasource",
		MaxOpenConns: 10,
		Timeout:      30,
	})

	// Use the config in tests
	var result TestConfig
	_ = config.UnmarshalDriverOpts("test", &result)

	// result now contains the mocked configuration
}
