/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	commondig "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
)

// outerSDK stands for an application SDK that wraps the view SDK, as
// fabricsdk.NewFrom(viewsdk.NewSDK(...)) does, and provides a store opener.
type outerSDK struct {
	commondig.SDK
	opened *int
	err    error
}

func (s *outerSDK) Install() error {
	if err := s.Container().Provide(func() StoreOpenerEntry {
		return StoreOpenerEntry{Opener: StoreOpenerFunc(func(context.Context) error { *s.opened++; return s.err })}
	}); err != nil {
		return err
	}
	return s.SDK.Install()
}

// storeOpeners calls an opener that an SDK wrapping the view SDK provides, and
// returns its error.
func TestStoreOpenersReachNestedSDK(t *testing.T) {
	t.Parallel()

	for _, want := range []error{nil, errors.New("open failed")} {
		c := NewContainer()
		var opened int
		inner := commondig.NewBaseSDK(c, &mockConfigService{})
		outer := &outerSDK{SDK: inner, opened: &opened, err: want}
		require.NoError(t, outer.Install())

		err := storeOpeners{c: c}.OpenStores(t.Context())
		if want == nil {
			require.NoError(t, err)
		} else {
			require.ErrorIs(t, err, want)
		}
		require.Equal(t, 1, opened)
	}
}

// blockingStore has one step that blocks until release is closed.
type blockingStore struct{ running, release chan struct{} }

func (blockingStore) StoreName() string   { return "readyz_block" }
func (blockingStore) CreateSchema() error { return nil }
func (s blockingStore) Steps() []migrate.Step {
	return []migrate.Step{{Version: 1, Name: "block", Go: func(context.Context, migrate.Execer) error {
		close(s.running)
		<-s.release
		return nil
	}}}
}

// /readyz answers 503 with reason "migrating" while a store runs schema steps,
// and the gate stops reporting once the migration returns. Not parallel: Go
// runs it before the parallel tests, so no other store migrates meanwhile.
func TestReadyzMigrating(t *testing.T) { //nolint:paralleltest // reads package-level migration state
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "m.db"))
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	ws := &recordingWebServer{}
	mountHealth(ws, connstate.NewMetrics(&disabled.Provider{}))
	get := func() (int, string) {
		rec := httptest.NewRecorder()
		ws.handlers["/readyz"].ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		return rec.Code, rec.Body.String()
	}

	s := blockingStore{running: make(chan struct{}), release: make(chan struct{})}
	done := make(chan error, 1)
	go func() { done <- migrate.Migrate(t.Context(), db, migrate.SQLite, "", s) }()
	<-s.running
	code, body := get()
	require.Equal(t, http.StatusServiceUnavailable, code)
	require.JSONEq(t, `{"status":"not ready","reason":"migrating"}`, body)

	close(s.release)
	require.NoError(t, <-done)
	code, _ = get()
	require.Equal(t, http.StatusOK, code)
}

// storeOpeners calls every opener even if one fails, and joins their errors.
func TestStoreOpenersReportEveryStore(t *testing.T) {
	t.Parallel()
	c := NewContainer()
	calls := 0
	for _, err := range []error{migrate.ErrPending, migrate.ErrSchemaTooNew} {
		require.NoError(t, c.Provide(func() StoreOpenerEntry {
			return StoreOpenerEntry{Opener: StoreOpenerFunc(func(context.Context) error { calls++; return err })}
		}))
	}
	err := storeOpeners{c: c}.OpenStores(t.Context())
	require.Equal(t, 2, calls)
	require.ErrorIs(t, err, migrate.ErrPending)
	require.ErrorIs(t, err, migrate.ErrSchemaTooNew)
}
