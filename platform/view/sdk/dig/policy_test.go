/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	commondig "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server"
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

// policyCheckers accepts only if every checker does, and runs none after the first refusal.
func TestPolicyCheckers(t *testing.T) {
	t.Parallel()
	signed := &protos.SignedCommand{Command: []byte("cmd"), Signature: []byte("sig")}
	command := &protos.Command{Header: &protos.Header{Creator: []byte("client")}}
	denied := errors.Wrap(view.ErrDenied, "view [admin] not allowed")

	for _, tc := range []struct {
		name  string
		errs  []error
		calls []int // how often each checker runs
		want  error
	}{
		{name: "both allow", errs: []error{nil, nil}, calls: []int{1, 1}},
		{name: "first refuses", errs: []error{denied, nil}, calls: []int{1, 0}, want: denied},
		{name: "second refuses", errs: []error{nil, denied}, calls: []int{1, 1}, want: denied},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var cs policyCheckers
			for _, err := range tc.errs {
				cs = append(cs, &stubChecker{err: err})
			}
			err := cs.Check(signed, command)

			var calls []int
			for _, c := range cs {
				calls = append(calls, c.(*stubChecker).calls)
			}
			require.Equal(t, tc.calls, calls)
			require.Equal(t, tc.want, err)
		})
	}
}

// policySDK stands for an application SDK that wraps the view SDK and provides a policy checker.
type policySDK struct{ commondig.SDK }

func (s *policySDK) Install() error {
	if err := s.Container().Provide(func() PolicyCheckerEntry {
		return PolicyCheckerEntry{Checker: server.YesPolicyChecker{}}
	}); err != nil {
		return err
	}
	return s.SDK.Install()
}

// The view service's policy checker runs the access control checker first, then the checkers
// that SDKs wrapping the view SDK provide; with none provided, it is the access control
// checker alone.
func TestNewPolicyChecker(t *testing.T) {
	t.Parallel()
	ac := &server.AccessControlChecker{}
	for _, tc := range []struct {
		name string
		sdk  func(commondig.SDK) commondig.SDK
		want server.PolicyChecker
	}{
		{name: "no group checker", sdk: func(s commondig.SDK) commondig.SDK { return s }, want: policyCheckers{ac}},
		{name: "nested SDK", sdk: func(s commondig.SDK) commondig.SDK { return &policySDK{SDK: s} }, want: policyCheckers{ac, server.YesPolicyChecker{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := NewContainer()
			require.NoError(t, tc.sdk(commondig.NewBaseSDK(c, &mockConfigService{})).Install())
			require.NoError(t, c.Provide(func() *server.AccessControlChecker { return ac }))
			require.NoError(t, c.Provide(newPolicyChecker))

			require.NoError(t, c.Invoke(func(pc server.PolicyChecker) { require.Equal(t, tc.want, pc) }))
		})
	}
}
