/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"context"
	stderrors "errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

func TestClassify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want failureClass
	}{
		{"nil is none", nil, classNone},
		{"deadlock is retryable", dbdriver.DeadlockDetected, classRetry},
		{"sql busy is retryable", dbdriver.SqlBusy, classRetry},
		{"deadline exceeded is retryable", context.DeadlineExceeded, classRetry},
		{"canceled is retryable", context.Canceled, classRetry},
		{"config rejected degrades", driver.ErrConfigRejected, classDegrade},
		{"fatal is fatal", ErrFatalCommit, classFatal},
		{"unknown error degrades", stderrors.New("boom"), classDegrade},
		// commitTxs and the handlers wrap every error on the way up, so
		// classification has to survive wrapping or production would only ever
		// see classDegrade.
		{"wrapped deadlock is retryable", errors.Wrapf(dbdriver.DeadlockDetected, "failed committing tx"), classRetry},
		{"deeply wrapped deadlock is retryable", errors.Wrapf(errors.Wrapf(dbdriver.DeadlockDetected, "inner"), "outer"), classRetry},
		{"wrapped config rejection degrades", errors.Wrapf(driver.ErrConfigRejected, "failed updating membership"), classDegrade},
		{"wrapped fatal is fatal", errors.Wrapf(ErrFatalCommit, "vault unreconcilable"), classFatal},
		// Fatal outranks a retryable cause it happens to travel with, so a node
		// that cannot trust its state never spins retrying instead.
		{"fatal outranks retryable", errors.Wrapf(ErrFatalCommit, "after %v", dbdriver.DeadlockDetected), classFatal},
		{"retries exhausted degrades", ErrRetriesExhausted, classDegrade},
		// An exhausted budget keeps the transient error that caused it reachable,
		// which is what makes the check order in classify load-bearing: testing
		// contention before exhaustion would classify this retryable again and
		// re-commit the block forever.
		{
			"exhausted outranks the transient cause it carries",
			errors.Join(ErrRetriesExhausted, dbdriver.DeadlockDetected),
			classDegrade,
		},
		{
			"exhausted outranks a wrapped transient cause",
			errors.Wrapf(errors.Join(ErrRetriesExhausted, errors.Wrapf(dbdriver.SqlBusy, "still busy")), "block [1] failed"),
			classDegrade,
		},
		// Fatal still outranks exhaustion, so a node that cannot trust its state
		// is never merely degraded because the failure arrived on the last retry.
		{
			"fatal outranks exhausted",
			errors.Join(ErrRetriesExhausted, ErrFatalCommit),
			classFatal,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, classify(tc.err), "class for [%v]", tc.err)
		})
	}
}

func TestFailureClassString(t *testing.T) {
	t.Parallel()

	require.Equal(t, "none", classNone.String())
	require.Equal(t, "retry", classRetry.String())
	require.Equal(t, "degrade", classDegrade.String())
	require.Equal(t, "fatal", classFatal.String())
	require.Equal(t, "unknown", failureClass(99).String())
}
