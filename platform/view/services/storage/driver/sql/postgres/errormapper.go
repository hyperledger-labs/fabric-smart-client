/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"
	sqldriver "database/sql/driver"
	stderrors "errors"
	"io"
	"net"
	"slices"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

var errorMap = map[string]error{
	"23505": driver.UniqueKeyViolation,
	"40001": driver.SqlBusy, // serialization_failure
	"40P01": driver.DeadlockDetected,
	"08000": driver.ErrConnectionLost, // connection_exception
	"08001": driver.ErrConnectionLost, // sqlclient_unable_to_establish_sqlconnection
	"08003": driver.ErrConnectionLost, // connection_does_not_exist
	"08006": driver.ErrConnectionLost, // connection_failure
	"08007": driver.ErrConnectionLost, // transaction_resolution_unknown
	"57P01": driver.ErrConnectionLost, // admin_shutdown
	"57P02": driver.ErrConnectionLost, // crash_shutdown
	"57P03": driver.ErrConnectionLost, // cannot_connect_now
	// too_many_connections: the server is at max_connections, typically while
	// every client reconnects to a new primary after a failover, or at the
	// CONNECTION LIMIT of the role or database. The other class 53 codes report
	// a disk, memory or configuration limit hit by a running statement, which
	// reconnecting does not clear, so they stay unmapped.
	"53300": driver.ErrConnectionLost,
}

// ErrorMapper maps pgx errors onto the sentinels of the storage driver package,
// so that callers match them with errors.Is independently of the backend.
type ErrorMapper struct{}

// WrapError returns err wrapped around the matching sentinel, or err unchanged
// when none matches.
//
// A server error is matched by its SQLSTATE. An error the server did not report
// is driver.ErrConnectionLost when it is a failed connect, a closed or bad
// connection, a network error or an EOF on the socket; a failed connect
// includes DNS and TLS failures, which a retry clears only if their cause is
// transient.
//
// A failed connect joins the errors of every connect attempt, one or two per
// host of a multi-host data source. It is driver.ErrConnectionLost when any
// attempt reported a SQLSTATE that maps to driver.ErrConnectionLost, such as
// too_many_connections, because a later connect can reach the server that
// reported it. Otherwise its first SQLSTATE decides, so that a rejected
// password or a missing database is returned unchanged, because retrying it
// cannot succeed. An attempt without a SQLSTATE does not override another
// attempt's SQLSTATE: under sslmode=prefer, pgx records one for a TLS handshake
// the server refused before it retries the same host in plaintext.
//
// A cancelled context is returned unchanged, so that a caller shutting down is
// not told to retry. A failed connect is a lost connection even when it ended
// with context.DeadlineExceeded: pgx reports a connect_timeout that way, and
// because the connect_timeout context derives from the caller's, a caller
// deadline that expires while connecting produces the same error. A caller that
// owns a deadline checks its own context before retrying. Any other
// context.DeadlineExceeded is returned unchanged, although it is a net.Error,
// because the context expired and not the connection.
func (*ErrorMapper) WrapError(err error) error {
	if err == nil || stderrors.Is(err, context.Canceled) {
		return err
	}
	_, isConnect := stderrors.AsType[*pgconn.ConnectError](err)
	if isConnect && hasConnectionLostCode(err) {
		return errors.Wrapf(driver.ErrConnectionLost, "%s", err)
	}
	if pgErr, ok := stderrors.AsType[*pgconn.PgError](err); ok {
		mappedErr, ok := errorMap[pgErr.Code]
		if !ok {
			logger.Debugf("unmapped postgres error with code [%s]", pgErr.Code)
			return err
		}
		return errors.Wrapf(mappedErr, "%s", err)
	}
	if isConnect {
		return errors.Wrapf(driver.ErrConnectionLost, "%s", err)
	}
	if stderrors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if isConnectionLost(err) {
		return errors.Wrapf(driver.ErrConnectionLost, "%s", err)
	}
	logger.Debugf("error of type [%T] not pgError", err)
	return err
}

// hasConnectionLostCode reports whether any *pgconn.PgError in the tree of err
// carries a SQLSTATE that maps to driver.ErrConnectionLost. errors.As stops at
// the first match, which in a joined connect error is the first attempt's.
func hasConnectionLostCode(err error) bool {
	switch e := err.(type) { //nolint:errorlint // walks the tree itself; errors.As stops at the first match
	case *pgconn.PgError:
		return stderrors.Is(errorMap[e.Code], driver.ErrConnectionLost)
	case interface{ Unwrap() []error }:
		return slices.ContainsFunc(e.Unwrap(), hasConnectionLostCode)
	case interface{ Unwrap() error }:
		return hasConnectionLostCode(e.Unwrap())
	}
	return false
}

func isConnectionLost(err error) bool {
	var netErr net.Error
	return stderrors.As(err, &netErr) ||
		stderrors.Is(err, pgconn.ErrConnClosed) ||
		stderrors.Is(err, sqldriver.ErrBadConn) ||
		stderrors.Is(err, io.EOF) ||
		stderrors.Is(err, io.ErrUnexpectedEOF)
}
