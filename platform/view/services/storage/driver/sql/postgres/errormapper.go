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
// A server error is matched by its SQLSTATE. An error the server did not report
// is driver.ErrConnectionLost when it is a failed connect, a closed or bad
// connection, a network error or an EOF on the socket; a failed connect includes
// DNS and TLS failures, which a retry clears only if their cause is transient. A
// context error is returned unchanged, although context.DeadlineExceeded is a
// net.Error, because the context expired and not the connection.
func (*ErrorMapper) WrapError(err error) error {
	if err == nil || stderrors.Is(err, context.DeadlineExceeded) || stderrors.Is(err, context.Canceled) {
		return err
	}
	var pgErr *pgconn.PgError
	if !stderrors.As(err, &pgErr) {
		if isConnectionLost(err) {
			return errors.Wrapf(driver.ErrConnectionLost, "%s", err)
		}
		logger.Debugf("error of type [%T] not pgError", err)
		return err
	}
	mappedErr, ok := errorMap[pgErr.Code]
	if !ok {
		logger.Debugf("unmapped postgres error with code [%s]", pgErr.Code)
		return err
	}
	return errors.Wrapf(mappedErr, "%s", err)
}

func isConnectionLost(err error) bool {
	var connectErr *pgconn.ConnectError
	var netErr net.Error
	return stderrors.As(err, &connectErr) ||
		stderrors.As(err, &netErr) ||
		stderrors.Is(err, pgconn.ErrConnClosed) ||
		stderrors.Is(err, sqldriver.ErrBadConn) ||
		stderrors.Is(err, io.EOF) ||
		stderrors.Is(err, io.ErrUnexpectedEOF)
}
