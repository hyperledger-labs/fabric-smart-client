/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	testing2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/testing"
	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestPostgres(t *testing.T) {
	if os.Getenv("TEST_POSTGRES") != "true" {
		t.Skip("set environment variable TEST_POSTGRES to true to include postgres test")
	}
	if testing.Short() {
		t.Skip("skipping postgres test in short mode")
	}
	t.Log("starting postgres")
	terminate, pgConnStr, err := StartPostgres(t, false)
	if err != nil {
		t.Fatal(err)
	}
	defer terminate()
	t.Log("postgres ready")

	cp := NewConfigProvider(testing2.MockConfig(Config{
		DataSource: pgConnStr,
	}))
	common3.TestCases(t, func(string) (driver.KeyValueStore, error) {
		return NewPersistenceWithOpts(cp, NewDbProvider(), "", NewKeyValueStore)
	}, func(string) (driver.UnversionedNotifier, error) {
		return NewPersistenceWithOpts(cp, NewDbProvider(), "", func(dbs *common.RWDB, tables common3.TableNames) (*KeyValueStoreNotifier, error) {
			return &KeyValueStoreNotifier{
				KeyValueStore: newKeyValueStore(dbs.ReadDB, dbs.WriteDB, tables.KVS),
				Notifier:      NewNotifier(dbs.WriteDB, tables.KVS, pgConnStr, AllOperations, primaryKey{"ns", identity}, primaryKey{"pkey", decode}),
			}, nil

		})
	}, func(p driver.KeyValueStore) *common3.KeyValueStore {
		return p.(*KeyValueStore).KeyValueStore
	})
}

func verifyExtendedLoggingInCommand(t *testing.T, config *ContainerConfig, expectExtendedLogging bool) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err)

	ctx := context.Background()
	containers, err := cli.ContainerList(ctx, container.ListOptions{})
	require.NoError(t, err)

	// Find our container
	var foundContainer *types.Container
	for _, c := range containers {
		for _, name := range c.Names {
			if name == "/"+config.Container {
				foundContainer = &c
				break
			}
		}
	}
	require.NotNil(t, foundContainer, "Container not found")

	// Inspect the container to get the command
	inspect, err := cli.ContainerInspect(ctx, foundContainer.ID)
	require.NoError(t, err)

	// Check extended logging parameters based on expectation
	cmd := inspect.Config.Cmd
	if expectExtendedLogging {
		assert.Contains(t, cmd, "log_destination=stderr")
		assert.Contains(t, cmd, "logging_collector=on")
		assert.Contains(t, cmd, "log_min_duration_statement=0")
		assert.Contains(t, cmd, "log_connections=on")
		assert.Contains(t, cmd, "log_disconnections=on")
	} else {
		// When extended logging is disabled, these parameters should not be present
		assert.NotContains(t, cmd, "log_destination=stderr")
		assert.NotContains(t, cmd, "logging_collector=on")
		assert.NotContains(t, cmd, "log_min_duration_statement=0")
		assert.NotContains(t, cmd, "log_connections=on")
		assert.NotContains(t, cmd, "log_disconnections=on")
	}
}

func TestContainerStartup_ExtendedLogging(t *testing.T) {
	if os.Getenv("TEST_POSTGRES") != "true" {
		t.Skip("set environment variable TEST_POSTGRES to true to include postgres test")
	}
	if testing.Short() {
		t.Skip("skipping postgres container test in short mode")
	}

	t.Run("Container starts successfully with extended logging enabled", func(t *testing.T) {
		config := DefaultConfig("issuer-extended")
		config.DbConfig.ExtendedLogging = true

		closeFunc, err := startPostgresWithLogger(*config, t, false)
		assert.NoError(t, err)
		assert.NotNil(t, closeFunc)

		// Verify the container command includes extended logging parameters
		verifyExtendedLoggingInCommand(t, config, true)

		if closeFunc != nil {
			closeFunc()
		}
	})

	t.Run("Container starts successfully with extended logging disabled", func(t *testing.T) {
		config := DefaultConfig("issuer-normal")
		config.DbConfig.ExtendedLogging = false

		closeFunc, err := startPostgresWithLogger(*config, t, false)
		assert.NoError(t, err)
		assert.NotNil(t, closeFunc)

		// Verify the container command does not include extended logging parameters
		verifyExtendedLoggingInCommand(t, config, false)

		if closeFunc != nil {
			closeFunc()
		}
	})
}
