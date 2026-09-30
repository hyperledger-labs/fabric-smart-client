/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
)

const idemixTestDir = "../generic/msp/idemix/testdata/idemix"

func newIdemixMSP(t *testing.T, conf *msp.MSPConfig, version MSPVersion) MSP {
	t.Helper()
	thisMSP, err := New(&IdemixNewOpts{NewBaseOpts{Version: version}}, nil)
	require.NoError(t, err)
	require.NoError(t, thisMSP.Setup(conf))
	return thisMSP
}

func newLocalIdemixMSP(t *testing.T) MSP {
	t.Helper()
	conf, err := GetLocalMspConfigWithType(idemixTestDir, nil, "idemix", ProviderTypeToString(IDEMIX))
	require.NoError(t, err)
	return newIdemixMSP(t, conf, MSPv1_3)
}

func TestIdemixMSPVersion(t *testing.T) { //nolint:paralleltest
	conf, err := GetLocalMspConfigWithType(idemixTestDir, nil, "idemix", ProviderTypeToString(IDEMIX))
	require.NoError(t, err)

	for _, tc := range []struct {
		version, expected MSPVersion
	}{
		{MSPv1_1, MSPv1_1},
		{MSPv1_3, MSPv1_3},
		{MSPv1_4_3, MSPv1_3},
	} {
		thisMSP := newIdemixMSP(t, conf, tc.version)
		require.Equal(t, tc.expected, thisMSP.GetVersion())
		require.Equal(t, IDEMIX, thisMSP.GetType())
	}
}

func TestIdemixMSPIdentities(t *testing.T) { //nolint:paralleltest
	thisMSP := newLocalIdemixMSP(t)

	signer, err := thisMSP.GetDefaultSigningIdentity()
	require.NoError(t, err)
	require.IsType(t, &idemixSigningIdentityWrapper{}, signer)
	require.Equal(t, "idemix", signer.GetIdentifier().Mspid)
	require.NotEmpty(t, signer.GetOrganizationalUnits())

	pub := signer.GetPublicVersion()
	require.IsType(t, &idemixIdentityWrapper{}, pub)
	require.Equal(t, signer.GetIdentifier(), pub.GetIdentifier())
	require.Equal(t, signer.GetOrganizationalUnits(), pub.GetOrganizationalUnits())

	raw, err := pub.Serialize()
	require.NoError(t, err)
	id, err := thisMSP.DeserializeIdentity(raw)
	require.NoError(t, err)
	require.IsType(t, &idemixIdentityWrapper{}, id)
	require.Equal(t, pub.GetIdentifier(), id.GetIdentifier())

	sID := &msp.SerializedIdentity{}
	require.NoError(t, proto.Unmarshal(raw, sID))
	internal, err := thisMSP.(*idemixMSPWrapper).deserializeIdentityInternal(sID.IdBytes)
	require.NoError(t, err)
	require.IsType(t, &idemixIdentityWrapper{}, internal)
	require.Equal(t, pub.GetIdentifier(), internal.GetIdentifier())

	_, err = thisMSP.DeserializeIdentity([]byte("garbage"))
	require.Error(t, err)
	_, err = thisMSP.(*idemixMSPWrapper).deserializeIdentityInternal([]byte("garbage"))
	require.Error(t, err)

	principalBytes, err := proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: "idemix"})
	require.NoError(t, err)
	principal := &msp.MSPPrincipal{PrincipalClassification: msp.MSPPrincipal_ROLE, Principal: principalBytes}
	require.NoError(t, thisMSP.Validate(id))
	require.NoError(t, thisMSP.SatisfiesPrincipal(id, principal))

	x509ID := getIdentity(t, signcerts)
	require.ErrorContains(t, thisMSP.Validate(x509ID), "unexpected identity type")
	require.ErrorContains(t, thisMSP.SatisfiesPrincipal(x509ID, principal), "unexpected identity type")
}

func TestIdemixMSPWithoutSigner(t *testing.T) { //nolint:paralleltest
	dir := t.TempDir()
	require.NoError(t, os.CopyFS(filepath.Join(dir, "msp"), os.DirFS(filepath.Join(idemixTestDir, "msp"))))

	conf, err := GetVerifyingMspConfig(dir, "idemix", ProviderTypeToString(IDEMIX))
	require.NoError(t, err)
	thisMSP := newIdemixMSP(t, conf, MSPv1_3)

	_, err = thisMSP.GetDefaultSigningIdentity()
	require.Error(t, err)
}
