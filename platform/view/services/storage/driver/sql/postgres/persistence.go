/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/XSAM/otelsql"
	_ "github.com/jackc/pgx/v5/stdlib"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/lazy"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	sqlcommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
)

var logger = logging.MustGetLogger()

const driverName = "pgx"

// DefaultStartupConnectTimeout is how long Open waits for the database to become
// reachable when Opts.StartupConnectTimeout is not set.
const DefaultStartupConnectTimeout = 60 * time.Second

// DbProvider returns the connection pool for a set of Opts. Implementations share one
// pool among all callers that pass the same DataSource.
type DbProvider interface {
	Get(Opts) (*common.RWDB, error)
}

// NewDbProvider returns a DbProvider that opens each pool with Open and reports no metrics.
func NewDbProvider() DbProvider { return lazy.NewProviderWithKeyMapper(key, Open) }

// NewDbProviderWithMetrics returns a DbProvider that opens each pool with Open and then
// monitors it for the lifetime of the process: it reports the pool's reachability as the
// database component of the connection-state metrics and exports its pool statistics.
func NewDbProviderWithMetrics(m *connstate.Metrics, p metrics.Provider) DbProvider {
	return lazy.NewProviderWithKeyMapper(key, func(o Opts) (*common.RWDB, error) {
		dbs, err := Open(o)
		if err != nil {
			return nil, err
		}
		label := dataSourceLabel(o.DataSource)
		// Pools are never closed, so the monitor runs until the process exits.
		go monitorDB(context.Background(), dbs.WriteDB, label, defaultPingInterval, m.Tracker("", "", connstate.ComponentDatabase, label), p)
		return dbs, nil
	})
}

func key(o Opts) string { return o.DataSource }

// Open opens a connection pool for opts.DataSource and waits until the database answers a
// ping, so a node that starts together with its database does not fail. It retries with
// jittered exponential backoff for opts.StartupConnectTimeout (DefaultStartupConnectTimeout
// if zero or negative), logs each failed attempt, and then returns an error. Logs and the error
// identify the database by its data-source label, never by the DSN. ReadDB and WriteDB share
// the same pool.
func Open(opts Opts) (*common.RWDB, error) {
	db, err := sqlOpen(opts.DataSource, opts.Tracing)
	if err != nil {
		return nil, errors.Wrapf(err, "can't open %s database", driverName)
	}

	db.SetMaxOpenConns(opts.MaxOpenConns)
	db.SetMaxIdleConns(opts.MaxIdleConns)
	db.SetConnMaxIdleTime(opts.MaxIdleTime)

	timeout := opts.StartupConnectTimeout
	if timeout <= 0 {
		timeout = DefaultStartupConnectTimeout
	}
	label := dataSourceLabel(opts.DataSource)
	if err := waitForDB(db, label, timeout); err != nil {
		_ = db.Close()
		return nil, err
	}
	logger.Debugf("connected to [%s] at [%s], max open connections: %d, max idle connections: %d, max idle time: %v", driverName, label, opts.MaxOpenConns, opts.MaxIdleConns, opts.MaxIdleTime)

	return &common.RWDB{
		ReadDB:  db,
		WriteDB: db,
	}, nil
}

// pingAttemptTimeout bounds a single ping, so a dial to a host that drops packets fails
// and is retried instead of consuming the whole startup timeout.
const pingAttemptTimeout = 5 * time.Second

// waitForDB pings db until it answers or timeout elapses.
func waitForDB(db *sql.DB, label string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	attempt := 0
	err := utils.NewBackoffRetryRunner(utils.Infinitely, 100*time.Millisecond, 5*time.Second).RunContext(ctx, func() error {
		attempt++
		pingCtx, cancel := context.WithTimeout(ctx, pingAttemptTimeout)
		err := db.PingContext(pingCtx)
		cancel()
		if err != nil {
			logger.Warnf("database [%s] not reachable (attempt %d): %v", label, attempt, err)
		}
		return err
	})
	return errors.Wrapf(err, "database [%s] not reachable within %v", label, timeout)
}

func sqlOpen(dataSourceName string, tracing *sqlcommon.TracingConfig) (*sql.DB, error) {
	if tracing == nil {
		return sql.Open(driverName, dataSourceName)
	}
	return otelsql.Open(driverName, dataSourceName,
		otelsql.WithAttributes(semconv.DBSystemPostgreSQL),
	)
}

// Opts are the connection options Open uses for one pool.
type Opts struct {
	DataSource      string
	MaxOpenConns    int
	MaxIdleConns    int
	MaxIdleTime     time.Duration
	TablePrefix     string
	TableNameParams []string
	Tracing         *sqlcommon.TracingConfig
	// StartupConnectTimeout bounds how long Open waits for the database; zero or negative
	// means DefaultStartupConnectTimeout.
	StartupConnectTimeout time.Duration
}
