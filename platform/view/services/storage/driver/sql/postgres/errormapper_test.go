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
	"syscall"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

func TestErrorMapper_ConnectionLossCodes(t *testing.T) {
	t.Parallel()

	pg := func(code string) error { return &pgconn.PgError{Code: code} }
	opErr := &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
	connectErr := func() error {
		// pgx returns *pgconn.ConnectError for a failed dial, which it builds
		// unexported; connecting to a closed port produces a real one.
		_, err := pgconn.Connect(t.Context(), "postgres://u:p@127.0.0.1:1/db?connect_timeout=1")
		require.Error(t, err)
		var ce *pgconn.ConnectError
		require.ErrorAs(t, err, &ce)
		return err
	}()
	// pgx reports a statement on a connection it already closed this way; it
	// is not a net.Error and wraps no I/O error.
	connClosed := errors.Wrapf(pgconn.ErrConnClosed, "commit")

	tests := []struct {
		name string
		err  error
		want error // nil: returned unchanged
	}{
		{"unique violation unchanged", pg("23505"), driver.UniqueKeyViolation},
		{"deadlock", pg("40P01"), driver.DeadlockDetected},
		{"serialization failure is busy", pg("40001"), driver.SqlBusy},
		{"connection exception", pg("08000"), driver.ErrConnectionLost},
		{"connection failure", pg("08006"), driver.ErrConnectionLost},
		{"connection does not exist", pg("08003"), driver.ErrConnectionLost},
		{"transaction resolution unknown", pg("08007"), driver.ErrConnectionLost},
		{"protocol violation is deterministic", pg("08P01"), nil},
		{"bad conn", sqldriver.ErrBadConn, driver.ErrConnectionLost},
		{"admin shutdown", pg("57P01"), driver.ErrConnectionLost},
		{"crash shutdown", pg("57P02"), driver.ErrConnectionLost},
		{"cannot connect now", pg("57P03"), driver.ErrConnectionLost},
		{"wrapped admin shutdown", errors.Wrapf(pg("57P01"), "failed setting tx to busy"), driver.ErrConnectionLost},
		{"net error", opErr, driver.ErrConnectionLost},
		{"wrapped net error", errors.Wrapf(opErr, "failed to initiate db transaction"), driver.ErrConnectionLost},
		{"connect error", connectErr, driver.ErrConnectionLost},
		{"conn closed", connClosed, driver.ErrConnectionLost},
		{"eof", io.EOF, driver.ErrConnectionLost},
		{"unexpected eof", io.ErrUnexpectedEOF, driver.ErrConnectionLost},
		{"unmapped code", pg("42P01"), nil},
		{"wrapped unmapped code keeps its context", errors.Wrapf(pg("42P01"), "failed writing state"), nil},
		{"unrelated error", stderrors.New("boom"), nil},
		// context.DeadlineExceeded is a net.Error; it must keep its own cause.
		{"deadline exceeded", context.DeadlineExceeded, nil},
		{"canceled", errors.Wrapf(context.Canceled, "query"), nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := (&ErrorMapper{}).WrapError(tc.err)
			if tc.want == nil {
				require.Equal(t, tc.err, got)
				require.NotErrorIs(t, got, driver.ErrConnectionLost)
				return
			}
			require.ErrorIs(t, got, tc.want)
			require.Contains(t, got.Error(), tc.err.Error(), "the original message is kept")
		})
	}

	require.NoError(t, (&ErrorMapper{}).WrapError(nil))
}
