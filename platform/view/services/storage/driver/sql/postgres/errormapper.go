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
}

// ErrorMapper maps pgx errors onto the sentinels of the storage driver package,
// so that callers match them with errors.Is independently of the backend.
type ErrorMapper struct{}

// WrapError returns err wrapped around the matching sentinel, or err unchanged
// when none matches.
//
// A server error is matched by its SQLSTATE, also when the server reported it
// while connecting: a rejected password or a missing database is returned
// unchanged, because retrying it cannot succeed. An error the server did not
// report is driver.ErrConnectionLost when it is a failed connect, a closed or
// bad connection, a network error or an EOF on the socket; a failed connect
// includes DNS and TLS failures, which a retry clears only if their cause is
// transient.
//
// A cancelled context is returned unchanged, so that a caller shutting down is
// not told to retry. A failed connect is a lost connection even when it ended
// with context.DeadlineExceeded: pgx reports a connect_timeout that way, and
// because the connect_timeout context derives from the caller's, an expired
// caller deadline produces the same error. A caller that owns a deadline checks
// its own context before retrying. Any other context.DeadlineExceeded is
// returned unchanged, although it is a net.Error, because the context expired
// and not the connection.
func (*ErrorMapper) WrapError(err error) error {
	if err == nil || stderrors.Is(err, context.Canceled) {
		return err
	}
	if pgErr, ok := stderrors.AsType[*pgconn.PgError](err); ok {
		mappedErr, ok := errorMap[pgErr.Code]
		if !ok {
			logger.Debugf("unmapped postgres error with code [%s]", pgErr.Code)
			return err
		}
		return errors.Wrapf(mappedErr, "%s", err)
	}
	if _, ok := stderrors.AsType[*pgconn.ConnectError](err); ok {
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

func isConnectionLost(err error) bool {
	var netErr net.Error
	return stderrors.As(err, &netErr) ||
		stderrors.Is(err, pgconn.ErrConnClosed) ||
		stderrors.Is(err, sqldriver.ErrBadConn) ||
		stderrors.Is(err, io.EOF) ||
		stderrors.Is(err, io.ErrUnexpectedEOF)
}
