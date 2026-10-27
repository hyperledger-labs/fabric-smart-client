/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"cmp"
	"context"
	"database/sql"
	sqldriver "database/sql/driver"
	stderrors "errors"
	"io"
	"net"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgproto3"
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
		{"too many connections", pg("53300"), driver.ErrConnectionLost},
		{"disk full is deterministic", pg("53100"), nil},
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

// TestErrorMapper_Connect opens connections through database/sql, as the stores
// do, to a listener that accepts TCP but never answers the startup message, the
// way a frozen or partitioned primary behaves, and to fake servers that reject
// the login with a SQLSTATE.
func TestErrorMapper_Connect(t *testing.T) {
	t.Parallel()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })
	silent := "postgres://u:p@" + l.Addr().String() + "/db?sslmode=disable"

	ping := func(t *testing.T, ctx context.Context, dsn string) error {
		t.Helper()
		db, err := sql.Open(driverName, dsn)
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })
		err = db.PingContext(ctx)
		require.Error(t, err)
		return (&ErrorMapper{}).WrapError(err)
	}

	t.Run("connect_timeout is a lost connection", func(t *testing.T) {
		t.Parallel()
		got := ping(t, t.Context(), silent+"&connect_timeout=1")
		require.ErrorIs(t, got, driver.ErrConnectionLost)
	})

	t.Run("one host timed out, the next refused", func(t *testing.T) {
		t.Parallel()
		got := ping(t, t.Context(), "postgres://u:p@"+l.Addr().String()+",127.0.0.1:1/db?sslmode=disable&connect_timeout=1")
		require.ErrorIs(t, got, driver.ErrConnectionLost)
	})

	// pgx reports the caller's deadline and connect_timeout with the same error,
	// because the connect_timeout context derives from the caller's. A caller
	// that owns the deadline checks its own context, as retryBlock does.
	t.Run("caller deadline is a lost connection too", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		got := ping(t, ctx, silent)
		require.ErrorIs(t, got, driver.ErrConnectionLost)
	})

	t.Run("caller cancel stays a context error", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		time.AfterFunc(200*time.Millisecond, cancel)
		got := ping(t, ctx, silent)
		require.ErrorIs(t, got, context.Canceled)
		require.NotErrorIs(t, got, driver.ErrConnectionLost)
	})

	// A server that rejects the login answers with a SQLSTATE inside the
	// connect error; retrying cannot fix a wrong password.
	t.Run("rejected password is not a lost connection", func(t *testing.T) {
		t.Parallel()
		got := ping(t, t.Context(), "postgres://u:p@"+rejectingServer(t, "28P01")+"/db?sslmode=disable&connect_timeout=5")
		var pgErr *pgconn.PgError
		require.ErrorAs(t, got, &pgErr)
		require.Equal(t, "28P01", pgErr.Code)
		require.NotErrorIs(t, got, driver.ErrConnectionLost)
	})

	// A server at max_connections rejects the startup message with 53300, which
	// is common right after a failover, when every client reconnects at once.
	t.Run("too many connections is a lost connection", func(t *testing.T) {
		t.Parallel()
		got := ping(t, t.Context(), "postgres://u:p@"+rejectingServer(t, "53300")+"/db?sslmode=disable&connect_timeout=5")
		require.ErrorIs(t, got, driver.ErrConnectionLost)
	})

	// pgx joins the errors of every attempt into one connect error. A host that
	// reported a transient SQLSTATE can accept a later attempt, whatever the
	// other attempts reported and in whichever order the hosts are listed.
	// sslmode=prefer makes two attempts per host, and the refused TLS one
	// carries no SQLSTATE.
	t.Run("multi-host", func(t *testing.T) {
		t.Parallel()
		busy := rejectingServer(t, "53300")
		hbaRejected := rejectingServer(t, "28000") // pgx tries the next host after it
		for _, tc := range []struct {
			name    string
			hosts   string
			sslmode string // empty: disable
			lost    bool
		}{
			{name: "too many connections, then timed out", hosts: busy + "," + l.Addr().String(), lost: true},
			{name: "timed out, then too many connections", hosts: l.Addr().String() + "," + busy, lost: true},
			{name: "rejected, then too many connections", hosts: hbaRejected + "," + busy, lost: true},
			{name: "rejected, then timed out", hosts: hbaRejected + "," + l.Addr().String()},
			{name: "rejected by every host", hosts: hbaRejected + "," + hbaRejected},
			{name: "rejected after refused TLS", hosts: hbaRejected, sslmode: "prefer"},
			{name: "too many connections after refused TLS", hosts: busy, sslmode: "prefer", lost: true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				sslmode := cmp.Or(tc.sslmode, "disable")
				got := ping(t, t.Context(), "postgres://u:p@"+tc.hosts+"/db?connect_timeout=1&sslmode="+sslmode)
				if tc.lost {
					require.ErrorIs(t, got, driver.ErrConnectionLost)
					return
				}
				var pgErr *pgconn.PgError
				require.ErrorAs(t, got, &pgErr)
				require.Equal(t, "28000", pgErr.Code)
				require.NotErrorIs(t, got, driver.ErrConnectionLost)
			})
		}
	})
}

// rejectingServer returns the address of a fake server that answers every
// startup message with a FATAL error carrying code, the way Postgres rejects a
// login.
func rejectingServer(t *testing.T, code string) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				backend := pgproto3.NewBackend(conn, conn)
				msg, err := backend.ReceiveStartupMessage()
				if err != nil {
					return
				}
				// Refuse TLS, as a server without ssl does, and read the
				// plaintext startup message that follows.
				if _, ok := msg.(*pgproto3.SSLRequest); ok {
					if _, err := conn.Write([]byte("N")); err != nil {
						return
					}
					if _, err := backend.ReceiveStartupMessage(); err != nil {
						return
					}
				}
				backend.Send(&pgproto3.ErrorResponse{Severity: "FATAL", Code: code, Message: "rejected"})
				_ = backend.Flush()
			}()
		}
	}()
	return l.Addr().String()
}
