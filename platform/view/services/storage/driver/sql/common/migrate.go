/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
)

// MigrationLockTimeoutKey bounds how long a node waits for the Postgres
// migration lock; unset or zero waits without limit. See migrate.WithLockTimeout.
const MigrationLockTimeoutKey = "fsc.storage.migrationLockTimeout"

// NodeConfig is the node-wide configuration MigrateOptions reads.
type NodeConfig interface {
	UnmarshalKey(key string, rawVal any) error
}

// MigrateOptions returns the migrate options of a SQL persistence: the node's
// FSC id (fsc.id) for the openers table, the lock timeout under
// MigrationLockTimeoutKey, and migrate.WithCheckOnly if skipCreateTable is set.
func MigrateOptions(c NodeConfig, skipCreateTable bool) ([]migrate.Option, error) {
	var (
		id      string
		timeout time.Duration
	)
	if err := c.UnmarshalKey("fsc.id", &id); err != nil {
		return nil, errors.Wrap(err, "failed to read fsc.id")
	}
	if err := c.UnmarshalKey(MigrationLockTimeoutKey, &timeout); err != nil {
		return nil, errors.Wrapf(err, "failed to read %s", MigrationLockTimeoutKey)
	}
	if timeout < 0 {
		return nil, errors.Errorf("%s must not be negative, got %s", MigrationLockTimeoutKey, timeout)
	}
	opts := []migrate.Option{migrate.WithNode(id), migrate.WithLockTimeout(timeout)}
	if skipCreateTable {
		opts = append(opts, migrate.WithCheckOnly())
	}
	return opts, nil
}
