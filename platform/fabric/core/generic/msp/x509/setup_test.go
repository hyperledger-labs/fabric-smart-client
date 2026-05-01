/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package x509

import (
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
)

func TestGetEnrollmentID(t *testing.T) {
	t.Parallel()
	_, certPEM := generateSelfSignedCert(t)
	raw := serializeIdentity(t, "msp1", certPEM)

	eid, err := GetEnrollmentID(raw)
	require.NoError(t, err)
	require.Equal(t, "test-user", eid)
}

func TestGetEnrollmentID_Invalid(t *testing.T) {
	t.Parallel()
	_, err := GetEnrollmentID([]byte("garbage"))
	require.Error(t, err)
}

func TestGetEnrollmentID_BadPEM(t *testing.T) {
	t.Parallel()
	si := serializeIdentity(t, "msp1", []byte("not pem"))
	_, err := GetEnrollmentID(si)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not PEM encoded")
}

func TestGetEnrollmentID_BadBlockType(t *testing.T) {
	t.Parallel()
	block := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("data")})
	si := serializeIdentity(t, "msp1", block)
	_, err := GetEnrollmentID(si)
	require.Error(t, err)
	require.Contains(t, err.Error(), "bad block type")
}

func TestGetRevocationHandle(t *testing.T) {
	t.Parallel()
	_, certPEM := generateSelfSignedCert(t)
	raw := serializeIdentity(t, "msp1", certPEM)

	rh, err := GetRevocationHandle(raw)
	require.NoError(t, err)
	require.NotEmpty(t, rh)
}

func TestGetRevocationHandle_Invalid(t *testing.T) {
	t.Parallel()
	_, err := GetRevocationHandle([]byte("garbage"))
	require.Error(t, err)
}

func TestGetRevocationHandle_BadPEM(t *testing.T) {
	t.Parallel()
	si := serializeIdentity(t, "msp1", []byte("not pem"))
	_, err := GetRevocationHandle(si)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not PEM encoded")
}

func TestGetRevocationHandle_BadBlockType(t *testing.T) {
	t.Parallel()
	block := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("data")})
	si := serializeIdentity(t, "msp1", block)
	_, err := GetRevocationHandle(si)
	require.Error(t, err)
	require.Contains(t, err.Error(), "bad block type")
}

func TestLoadLocalMSPSignerCert(t *testing.T) {
	t.Parallel()
	cert, err := LoadLocalMSPSignerCert("./testdata/msp")
	require.NoError(t, err)
	require.NotEmpty(t, cert)
}

func TestLoadLocalMSPSignerCert_InvalidDir(t *testing.T) {
	t.Parallel()
	_, err := LoadLocalMSPSignerCert("/nonexistent/path")
	require.Error(t, err)
}

func TestGetPemMaterialFromDir(t *testing.T) {
	t.Parallel()
	certs, err := getPemMaterialFromDir(filepath.Join("testdata", "msp", "cacerts"))
	require.NoError(t, err)
	require.NotEmpty(t, certs)
}

func TestGetPemMaterialFromDir_NonExistent(t *testing.T) {
	t.Parallel()
	_, err := getPemMaterialFromDir("/nonexistent/dir")
	require.Error(t, err)
}

func TestGetPemMaterialFromDir_EmptyDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	certs, err := getPemMaterialFromDir(dir)
	require.NoError(t, err)
	require.Empty(t, certs)
}

func TestGetPemMaterialFromDir_SkipsSubdirs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "subdir"), 0o755))
	certs, err := getPemMaterialFromDir(dir)
	require.NoError(t, err)
	require.Empty(t, certs)
}

func TestGetBCCSPFromConf_SW(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	csp, ks, err := GetBCCSPFromConf(dir, "", nil)
	require.NoError(t, err)
	require.NotNil(t, csp)
	require.NotNil(t, ks)
}

func TestGetBCCSPFromConf_InvalidDefault(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	conf := &config.BCCSP{Default: "INVALID"}
	_, _, err := GetBCCSPFromConf(dir, "", conf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid config.BCCSP.Default")
}

func TestGetSWBCCSP(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	csp, ks, err := GetSWBCCSP(dir)
	require.NoError(t, err)
	require.NotNil(t, csp)
	require.NotNil(t, ks)
}

func TestLoadLocalMSPAt_InvalidType(t *testing.T) {
	t.Parallel()
	_, err := LoadLocalMSPAt("./testdata/msp", "", "apple", "invalid", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid msp type")
}

func TestLoadVerifyingMSPAt_InvalidType(t *testing.T) {
	t.Parallel()
	_, err := LoadVerifyingMSPAt("./testdata/msp", "apple", "invalid")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid msp type")
}

func TestSerializeFromMSP(t *testing.T) {
	t.Parallel()
	serializesBCCSPFactoryState(t)
	raw, err := SerializeFromMSP("apple", "./testdata/msp")
	require.NoError(t, err)
	require.NotEmpty(t, raw)
}

func TestSerializeFromMSP_InvalidPath(t *testing.T) {
	t.Parallel()
	serializesBCCSPFactoryState(t)
	_, err := SerializeFromMSP("apple", "/nonexistent")
	require.Error(t, err)
}

func TestSkiMapper_KeyMapHit(t *testing.T) {
	t.Parallel()
	p11Opts := config.PKCS11{
		KeyIDs: []config.KeyIDMapping{
			{SKI: "abcd", ID: "mapped-key"},
		},
	}
	mapper := skiMapper(p11Opts)
	// hex.EncodeToString([]byte{0xab, 0xcd}) == "abcd"
	result := mapper([]byte{0xab, 0xcd})
	require.Equal(t, []byte("mapped-key"), result)
}

func TestSkiMapper_AltIDFallback(t *testing.T) {
	t.Parallel()
	p11Opts := config.PKCS11{
		AltID: "fallback-id",
	}
	mapper := skiMapper(p11Opts)
	result := mapper([]byte{0x01, 0x02})
	require.Equal(t, []byte("fallback-id"), result)
}

func TestSkiMapper_DefaultPassthrough(t *testing.T) {
	t.Parallel()
	p11Opts := config.PKCS11{}
	mapper := skiMapper(p11Opts)
	ski := []byte{0x01, 0x02, 0x03}
	result := mapper(ski)
	require.Equal(t, ski, result)
}

func TestGetPKCS11BCCSP_NilConfig(t *testing.T) {
	t.Parallel()
	conf := &config.BCCSP{
		Default: "PKCS11",
		PKCS11:  nil,
	}
	_, _, err := GetPKCS11BCCSP(conf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing configuration")
}

func TestGetBCCSPFromConf_ExplicitSW(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	conf := &config.BCCSP{Default: "SW"}
	csp, ks, err := GetBCCSPFromConf(dir, "", conf)
	require.NoError(t, err)
	require.NotNil(t, csp)
	require.NotNil(t, ks)
}

func TestGetBCCSPFromConf_PKCS11NilOpts(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	conf := &config.BCCSP{Default: "PKCS11", PKCS11: nil}
	_, _, err := GetBCCSPFromConf(dir, "", conf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing configuration")
}

func TestGetBCCSPFromConf_CustomKeyStorePath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ksDir := t.TempDir()
	csp, ks, err := GetBCCSPFromConf(dir, ksDir, nil)
	require.NoError(t, err)
	require.NotNil(t, csp)
	require.NotNil(t, ks)
}

func TestGetEnrollmentID_InvalidCert(t *testing.T) {
	t.Parallel()
	badCert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("bad der")})
	si := serializeIdentity(t, "msp1", badCert)

	_, err := GetEnrollmentID(si)
	require.Error(t, err)
	require.Contains(t, err.Error(), "pem bytes are not cert encoded")
}

func TestGetRevocationHandle_InvalidCert(t *testing.T) {
	t.Parallel()
	badCert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("bad der")})
	si := serializeIdentity(t, "msp1", badCert)

	_, err := GetRevocationHandle(si)
	require.Error(t, err)
	require.Contains(t, err.Error(), "pem bytes are not cert encoded")
}

func TestGetPemMaterialFromDir_SkipsNonPEM(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notpem.txt"), []byte("plain text"), 0o644))
	certs, err := getPemMaterialFromDir(dir)
	require.NoError(t, err)
	require.Empty(t, certs)
}

func TestGetSigningIdentity(t *testing.T) {
	t.Parallel()
	serializesBCCSPFactoryState(t)
	sID, err := GetSigningIdentity("./testdata/msp", "", "apple", nil)
	require.NoError(t, err)
	require.NotNil(t, sID)

	raw, err := sID.Serialize()
	require.NoError(t, err)
	require.NotEmpty(t, raw)
}

func TestGetSigningIdentity_InvalidPath(t *testing.T) {
	t.Parallel()
	serializesBCCSPFactoryState(t)
	_, err := GetSigningIdentity("/nonexistent", "", "msp1", nil)
	require.Error(t, err)
}

func TestLoadLocalMSPAt(t *testing.T) {
	t.Parallel()
	serializesBCCSPFactoryState(t)
	mspInst, err := LoadLocalMSPAt("./testdata/msp", "", "apple", BCCSPType, nil)
	require.NoError(t, err)
	require.NotNil(t, mspInst)
}

func TestLoadVerifyingMSPAt(t *testing.T) {
	t.Parallel()
	serializesBCCSPFactoryState(t)
	mspInst, err := LoadVerifyingMSPAt("./testdata/msp", "apple", BCCSPType)
	require.NoError(t, err)
	require.NotNil(t, mspInst)
}
