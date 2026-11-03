/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type fakeSignerService struct {
	GetSignerCount int
	LastError      error
	LastSigner     driver.Signer
	LastVerifier   driver.Verifier
	LastID         view.Identity
}

func (m *fakeSignerService) GetSigner(id view.Identity) (driver.Signer, error) {
	m.GetSignerCount++
	m.LastID = id
	return m.LastSigner, m.LastError
}

func (*fakeSignerService) AreMe(_ context.Context, _ ...view.Identity) []string {
	return nil
}

func (*fakeSignerService) IsMe(_ context.Context, _ view.Identity) bool {
	return false
}

func (*fakeSignerService) GetSigningIdentity(_ view.Identity) (cdriver.SigningIdentity, error) {
	return nil, nil
}

func (m *fakeSignerService) GetVerifier(id view.Identity) (cdriver.Verifier, error) {
	m.LastID = id
	return m.LastVerifier, m.LastError
}

type dummySigner struct{}

func (*dummySigner) Sign(_ []byte) ([]byte, error) {
	return nil, nil
}

func TestSignerService(t *testing.T) {
	t.Parallel()

	mss := &fakeSignerService{}
	ss := &SignerService{sigService: mss}

	mss.LastSigner = &dummySigner{}
	sig, err := ss.GetSigner([]byte("id1"))
	require.NoError(t, err)
	require.NotNil(t, sig)
	require.Equal(t, 1, mss.GetSignerCount)
	require.Equal(t, view.Identity("id1"), mss.LastID)

	mss.LastError = errors.New("sig err")
	_, err = ss.GetSigner([]byte("id2"))
	require.ErrorContains(t, err, "sig err")
}

type dummyVerifier struct{}

func (*dummyVerifier) Verify(_, _ []byte) error { return nil }

func TestSignerServiceGetVerifier(t *testing.T) {
	t.Parallel()

	mss := &fakeSignerService{LastVerifier: &dummyVerifier{}}
	ss := &SignerService{sigService: mss}

	v, err := ss.GetVerifier([]byte("id1"))
	require.NoError(t, err)
	require.Same(t, mss.LastVerifier, v)
	require.Equal(t, view.Identity("id1"), mss.LastID)

	mss.LastError = errors.New("verifier err")
	_, err = ss.GetVerifier([]byte("id2"))
	require.ErrorContains(t, err, "verifier err")
}

// *sig.Service is the signature service of every network; LookupVerifier relies
// on it implementing verifierLookup.
var _ verifierLookup = (*sig.Service)(nil)

type lookupSignerService struct {
	fakeSignerService
	looked view.Identity
}

func (m *lookupSignerService) LookupVerifier(id view.Identity) (cdriver.Verifier, error) {
	m.looked = id
	return m.LastVerifier, m.LastError
}

func TestSignerServiceLookupVerifier(t *testing.T) {
	t.Parallel()

	lss := &lookupSignerService{LastVerifier: &dummyVerifier{}}
	v, err := (&SignerService{sigService: lss}).LookupVerifier([]byte("id1"))
	require.NoError(t, err)
	require.Same(t, lss.LastVerifier, v)
	require.Equal(t, view.Identity("id1"), lss.looked)
	require.Nil(t, lss.LastID, "GetVerifier is not called")

	mss := &fakeSignerService{LastVerifier: &dummyVerifier{}}
	v, err = (&SignerService{sigService: mss}).LookupVerifier([]byte("id2"))
	require.NoError(t, err)
	require.Same(t, mss.LastVerifier, v)
	require.Equal(t, view.Identity("id2"), mss.LastID, "falls back to GetVerifier")
}
