/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/sig/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	protos2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type acIdentityProvider struct {
	defaultID view2.Identity
	clients   []view2.Identity
}

func (p *acIdentityProvider) DefaultIdentity() view2.Identity { return p.defaultID }
func (p *acIdentityProvider) Clients() []view2.Identity       { return p.clients }

type acVerifierProvider struct {
	verifier sig.Verifier
	err      error
}

func (p *acVerifierProvider) GetVerifier(view2.Identity) (sig.Verifier, error) {
	return p.verifier, p.err
}

func TestAccessControlChecker_Check(t *testing.T) {
	t.Parallel()

	ip := &acIdentityProvider{
		defaultID: view2.Identity("default"),
		clients:   []view2.Identity{view2.Identity("client1"), view2.Identity("client2")},
	}
	signed := &protos2.SignedCommand{Command: []byte("cmd"), Signature: []byte("sig")}
	command := func(creator string) *protos2.Command {
		return &protos2.Command{Header: &protos2.Header{Creator: []byte(creator)}}
	}

	for _, creator := range []string{"default", "client1", "client2"} {
		t.Run("accepts "+creator, func(t *testing.T) {
			t.Parallel()
			verifier := &mock.Verifier{}
			checker := NewAccessControlChecker(ip, &acVerifierProvider{verifier: verifier})

			require.NoError(t, checker.Check(signed, command(creator)))
			require.Equal(t, 1, verifier.VerifyCallCount())
			msg, sigma := verifier.VerifyArgsForCall(0)
			require.Equal(t, signed.Command, msg)
			require.Equal(t, signed.Signature, sigma)
		})
	}

	t.Run("accepts default identity without clients", func(t *testing.T) {
		t.Parallel()
		checker := NewAccessControlChecker(
			&acIdentityProvider{defaultID: view2.Identity("default")},
			&acVerifierProvider{verifier: &mock.Verifier{}},
		)
		require.NoError(t, checker.Check(signed, command("default")))
	})

	t.Run("rejects unknown creator", func(t *testing.T) {
		t.Parallel()
		verifier := &mock.Verifier{}
		checker := NewAccessControlChecker(ip, &acVerifierProvider{verifier: verifier})

		err := checker.Check(signed, command("stranger"))
		require.ErrorIs(t, err, view.ErrIdentityNotRecognized)
		require.Zero(t, verifier.VerifyCallCount())
	})

	t.Run("fails when verifier is unavailable", func(t *testing.T) {
		t.Parallel()
		checker := NewAccessControlChecker(ip, &acVerifierProvider{err: errors.New("no verifier")})

		err := checker.Check(signed, command("client1"))
		require.ErrorContains(t, err, "failed getting verifier")
		require.ErrorContains(t, err, "no verifier")
	})

	t.Run("fails on invalid signature", func(t *testing.T) {
		t.Parallel()
		verifier := &mock.Verifier{}
		verifier.VerifyReturns(errors.New("bad signature"))
		checker := NewAccessControlChecker(ip, &acVerifierProvider{verifier: verifier})

		err := checker.Check(signed, command("client1"))
		require.ErrorContains(t, err, "failed verifying signature")
		require.ErrorContains(t, err, "bad signature")
	})
}
