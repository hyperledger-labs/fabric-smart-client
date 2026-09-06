/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type mockSigningIdentity struct{}

func (m *mockSigningIdentity) Serialize() ([]byte, error) {
	return []byte("serialized"), nil
}

func (m *mockSigningIdentity) Sign(msg []byte) ([]byte, error) {
	return []byte("sig"), nil
}

func TestLocalMembership(t *testing.T) {
	t.Parallel()

	mockFNS := &mock.FabricNetworkService{}
	mockLM := &mock.LocalMembership{}
	mockFNS.LocalMembershipReturns(mockLM)

	lm := &LocalMembership{network: mockFNS}

	// RegisterIdemixMSP
	mockLM.RegisterIdemixMSPReturns(nil)
	require.NoError(t, lm.RegisterIdemixMSP("id", "path", "msp"))
	require.Equal(t, 1, mockLM.RegisterIdemixMSPCallCount())

	// RegisterX509MSP
	mockLM.RegisterX509MSPReturns(nil)
	require.NoError(t, lm.RegisterX509MSP("id2", "path2", "msp2"))
	require.Equal(t, 1, mockLM.RegisterX509MSPCallCount())

	// DefaultSigningIdentity
	msi := &mockSigningIdentity{}
	mockLM.DefaultSigningIdentityReturns(msi)
	require.Equal(t, msi, lm.DefaultSigningIdentity())
	require.Equal(t, 1, mockLM.DefaultSigningIdentityCallCount())

	// DefaultIdentity
	mockLM.DefaultIdentityReturns([]byte("def"))
	require.Equal(t, view.Identity("def"), lm.DefaultIdentity())
	require.Equal(t, 1, mockLM.DefaultIdentityCallCount())

	// IsMe
	ctx := t.Context()
	mockLM.IsMeReturns(true)
	require.True(t, lm.IsMe(ctx, []byte("me")))
	require.Equal(t, 1, mockLM.IsMeCallCount())

	// AnonymousIdentity
	mockLM.AnonymousIdentityReturns([]byte("anon"), nil)
	anon, err := lm.AnonymousIdentity()
	require.NoError(t, err)
	require.Equal(t, view.Identity("anon"), anon)

	// GetIdentityByID
	mockLM.GetIdentityByIDReturns([]byte("id3"), nil)
	id3, err := lm.GetIdentityByID("id3")
	require.NoError(t, err)
	require.Equal(t, view.Identity("id3"), id3)

	// Refresh
	mockLM.RefreshReturns(nil)
	require.NoError(t, lm.Refresh())
}

type mockMSPManagerInner struct {
	driver.MSPManager
	deserializeErr      error
	deserializeIdentity driver.MSPIdentity
}

func (m *mockMSPManagerInner) DeserializeIdentity(serializedIdentity []byte) (driver.MSPIdentity, error) {
	return m.deserializeIdentity, m.deserializeErr
}

type mockMSPIdentity struct {
	driver.MSPIdentity
	mspid string
}

func (m *mockMSPIdentity) GetMSPIdentifier() string {
	return m.mspid
}

func TestMSPManager(t *testing.T) {
	t.Parallel()

	mockCM := &mock.ChannelMembership{}
	mgr := &MSPManager{ch: mockCM}

	// GetMSPIDs
	mockCM.GetMSPIDsReturns([]string{"msp1", "msp2"}, nil)
	ids, err := mgr.GetMSPIDs()
	require.NoError(t, err)
	require.Equal(t, []string{"msp1", "msp2"}, ids)

	// IsValid
	mockCM.IsValidReturns(nil)
	require.NoError(t, mgr.IsValid([]byte("valid-id")))

	// GetMSPIdentifier
	mockMI := &mockMSPIdentity{mspid: "MyMSP"}
	mockMMI := &mockMSPManagerInner{deserializeIdentity: mockMI}
	mockCM.MSPManagerReturns(mockMMI)
	idstr, err := mgr.GetMSPIdentifier([]byte("some-id"))
	require.NoError(t, err)
	require.Equal(t, "MyMSP", idstr)

	// GetMSPIdentifier error
	mockMMI.deserializeErr = errors.New("deser err")
	_, err = mgr.GetMSPIdentifier([]byte("some-id"))
	require.ErrorContains(t, err, "deser err")

	// GetVerifier
	// Just test it doesn't panic and returns what it gets
	mockCM.GetVerifierReturns(nil, errors.New("no verifier"))
	_, err = mgr.GetVerifier([]byte("vid"))
	require.ErrorContains(t, err, "no verifier")
}

func TestIdentityInfo(t *testing.T) {
	t.Parallel()

	mockFNS := &mock.FabricNetworkService{}
	mockLM := &mock.LocalMembership{}
	mockFNS.LocalMembershipReturns(mockLM)
	lm := &LocalMembership{network: mockFNS}

	// Identity options
	opts, err := CompileIdentityOptions(WithIdemixEIDExtension(), WithAuditInfo([]byte("audit")))
	require.NoError(t, err)
	require.True(t, opts.IdemixEIDExtension)
	require.Equal(t, []byte("audit"), opts.AuditInfo)

	// GetIdentityInfoByLabel
	mockIInfo := &driver.IdentityInfo{
		ID:           "id1",
		EnrollmentID: "eid1",
		GetIdentity: func(opts *driver.IdentityOptions) (view.Identity, []byte, error) {
			return view.Identity("ident"), []byte("audit"), nil
		},
	}
	mockLM.GetIdentityInfoByLabelReturns(mockIInfo)

	iInfo := lm.GetIdentityInfoByLabel("mspType", "label")
	require.NotNil(t, iInfo)
	require.Equal(t, "id1", iInfo.ID)
	require.Equal(t, "eid1", iInfo.EnrollmentID)

	id, audit, err := iInfo.GetIdentity(WithIdemixEIDExtension())
	require.NoError(t, err)
	require.Equal(t, view.Identity("ident"), id)
	require.Equal(t, []byte("audit"), audit)

	// nil GetIdentityInfoByLabel
	mockLM.GetIdentityInfoByLabelReturns(nil)
	require.Nil(t, lm.GetIdentityInfoByLabel("x", "y"))

	// GetIdentityInfoByIdentity
	mockLM.GetIdentityInfoByIdentityReturns(mockIInfo)
	iInfo2 := lm.GetIdentityInfoByIdentity("mspType", []byte("ident"))
	require.NotNil(t, iInfo2)
	require.Equal(t, "id1", iInfo2.ID)

	id2, audit2, err := iInfo2.GetIdentity(WithAuditInfo(nil))
	require.NoError(t, err)
	require.Equal(t, view.Identity("ident"), id2)
	require.Equal(t, []byte("audit"), audit2)

	// nil GetIdentityInfoByIdentity
	mockLM.GetIdentityInfoByIdentityReturns(nil)
	require.Nil(t, lm.GetIdentityInfoByIdentity("x", []byte("y")))
}
