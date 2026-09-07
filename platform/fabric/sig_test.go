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

	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type fakeSignerService struct {
	GetSignerCount int
	LastError      error
	LastSigner     driver.Signer
	LastID         view.Identity
}

func (m *fakeSignerService) GetSigner(id view.Identity) (driver.Signer, error) {
	m.GetSignerCount++
	m.LastID = id
	return m.LastSigner, m.LastError
}

func (m *fakeSignerService) AreMe(ctx context.Context, identities ...view.Identity) []string {
	return nil
}

func (m *fakeSignerService) IsMe(ctx context.Context, id view.Identity) bool {
	return false
}

func (m *fakeSignerService) GetSigningIdentity(id view.Identity) (driver2.SigningIdentity, error) {
	return nil, nil
}

func (m *fakeSignerService) GetVerifier(id view.Identity) (driver2.Verifier, error) {
	return nil, nil
}

type dummySigner struct{}

func (d *dummySigner) Sign(message []byte) ([]byte, error) {
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
