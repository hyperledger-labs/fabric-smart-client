/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
)

// stubChecker returns err and counts its calls.
type stubChecker struct {
	err   error
	calls int
}

func (s *stubChecker) Check(*protos.SignedCommand, *protos.Command) error {
	s.calls++
	return s.err
}

func denied() error { return errors.Wrap(view.ErrDenied, "view [admin] not allowed") }

// A denial reaches the client as the response's error text, never runs the view, and is
// counted as reason "denied"; another policy error is not counted.
func TestServer_PolicyDenial(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		err     error
		counted bool
	}{
		{name: "denied", err: denied(), counted: true},
		{name: "unauthenticated", err: errors.Wrap(view.ErrIdentityNotRecognized, "stranger")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, calls := countingServer(t)
			srv.PolicyChecker = &stubChecker{err: tc.err}
			rejected := &mock.Counter{}
			rejected.WithReturns(rejected)
			srv.metrics = &Metrics{RequestsReceived: srv.metrics.RequestsReceived, RequestsCompleted: srv.metrics.RequestsCompleted, RequestsRejected: rejected}

			resp, err := srv.ProcessCommand(context.Background(), buildSignedCommand(t, nil))
			require.NoError(t, err)
			require.Equal(t, tc.err.Error(), string(resp.Response))

			stream := &fakeStreamServer{ctx: context.Background(), sc: buildSignedCommand(t, nil)}
			require.ErrorIs(t, srv.StreamCommand(stream), tc.err)
			require.Len(t, stream.sent, 1)
			require.Equal(t, tc.err.Error(), string(stream.sent[0].Response))

			require.Zero(t, calls.Load())
			if !tc.counted {
				require.Zero(t, rejected.WithCallCount())
				return
			}
			require.Equal(t, 2, rejected.WithCallCount())
			require.Equal(t, []string{"reason", "denied"}, rejected.WithArgsForCall(0))
			require.Equal(t, []string{"reason", "denied"}, rejected.WithArgsForCall(1))
		})
	}
}
