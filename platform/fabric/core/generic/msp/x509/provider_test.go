/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package x509

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	msp2 "github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
)

func TestProvider_IsRemote(t *testing.T) {
	t.Parallel()
	p, err := NewProvider("./testdata/msp", "", "apple", nil)
	require.NoError(t, err)
	require.False(t, p.IsRemote())
}

func TestProvider_DeserializeVerifier(t *testing.T) {
	t.Parallel()
	p, err := NewProvider("./testdata/msp", "", "apple", nil)
	require.NoError(t, err)

	certPEM, err := os.ReadFile(filepath.Join("testdata", "msp", "signcerts", "auditor.org1.example.com-cert.pem"))
	require.NoError(t, err)
	raw := serializeIdentity(t, "apple", certPEM)

	verifier, err := p.DeserializeVerifier(raw)
	require.NoError(t, err)
	require.NotNil(t, verifier)
}

func TestProvider_DeserializeVerifier_Invalid(t *testing.T) {
	t.Parallel()
	p, err := NewProvider("./testdata/msp", "", "apple", nil)
	require.NoError(t, err)

	_, err = p.DeserializeVerifier([]byte("garbage"))
	require.Error(t, err)
}

func TestProvider_DeserializeSigner(t *testing.T) {
	t.Parallel()
	p, err := NewProvider("./testdata/msp", "", "apple", nil)
	require.NoError(t, err)

	_, err = p.DeserializeSigner(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not supported")
}

func TestProvider_Info(t *testing.T) {
	t.Parallel()
	p, err := NewProvider("./testdata/msp", "", "apple", nil)
	require.NoError(t, err)

	certPEM, err := os.ReadFile(filepath.Join("testdata", "msp", "signcerts", "auditor.org1.example.com-cert.pem"))
	require.NoError(t, err)
	raw := serializeIdentity(t, "apple", certPEM)

	info, err := p.Info(raw, nil)
	require.NoError(t, err)
	require.Contains(t, info, "MSP.x509:")
	require.Contains(t, info, "apple")
}

func TestProvider_Info_Invalid(t *testing.T) {
	t.Parallel()
	p, err := NewProvider("./testdata/msp", "", "apple", nil)
	require.NoError(t, err)

	_, err = p.Info([]byte("garbage"), nil)
	require.Error(t, err)
}

func TestProvider_String(t *testing.T) {
	t.Parallel()
	p, err := NewProvider("./testdata/msp", "", "apple", nil)
	require.NoError(t, err)
	require.Contains(t, p.String(), "X509 Provider for EID")
}

func TestProvider_SerializedIdentity(t *testing.T) {
	t.Parallel()
	p, err := NewProvider("./testdata/msp", "", "apple", nil)
	require.NoError(t, err)

	sID, err := p.SerializedIdentity()
	require.NoError(t, err)
	require.NotNil(t, sID)
}

func TestProvider_InvalidPath(t *testing.T) {
	t.Parallel()
	_, err := NewProvider("/nonexistent/path", "", "mspid", nil)
	require.Error(t, err)
}

func TestProvider_DeserializeVerifier_BadKey(t *testing.T) {
	t.Parallel()
	p, err := NewProvider("./testdata/msp", "", "apple", nil)
	require.NoError(t, err)

	badKey := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte("bad")})
	sID := &msp2.SerializedIdentity{Mspid: "apple", IdBytes: badKey}
	raw, err := proto.Marshal(sID)
	require.NoError(t, err)

	_, err = p.DeserializeVerifier(raw)
	require.Error(t, err)
}

func TestProvider_Info_BadCert(t *testing.T) {
	t.Parallel()
	p, err := NewProvider("./testdata/msp", "", "apple", nil)
	require.NoError(t, err)

	badCert := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("bad")})
	sID := &msp2.SerializedIdentity{Mspid: "apple", IdBytes: badCert}
	raw, err := proto.Marshal(sID)
	require.NoError(t, err)

	_, err = p.Info(raw, nil)
	require.Error(t, err)
}

func TestNewProviderWithBCCSPConfig_VerifyOnly(t *testing.T) {
	t.Parallel()
	// Create a temp MSP dir with signcerts and cacerts but NO keystore
	dir := t.TempDir()
	mspDir := filepath.Join(dir, "msp")

	// Copy cacerts
	srcCaCert, err := os.ReadFile(filepath.Join("testdata", "msp", "cacerts", "ca.org1.example.com-cert.pem"))
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(mspDir, "cacerts"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(mspDir, "cacerts", "ca.pem"), srcCaCert, 0o644))

	// Copy signcerts
	srcSignCert, err := os.ReadFile(filepath.Join("testdata", "msp", "signcerts", "auditor.org1.example.com-cert.pem"))
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(mspDir, "signcerts"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(mspDir, "signcerts", "cert.pem"), srcSignCert, 0o644))

	// Copy admincerts
	require.NoError(t, os.MkdirAll(filepath.Join(mspDir, "admincerts"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(mspDir, "admincerts", "admin.pem"), srcSignCert, 0o644))

	// No keystore → should fall through to verify-only
	p, err := NewProviderWithBCCSPConfig(mspDir, "", "testmsp", nil, nil)
	require.NoError(t, err)
	require.True(t, p.IsRemote())
	require.NotEmpty(t, p.EnrollmentID())
}

func TestProvider_Identity_FullFlow(t *testing.T) {
	t.Parallel()
	p, err := NewProvider("./testdata/msp", "", "apple", nil)
	require.NoError(t, err)

	id, auditInfo, err := p.Identity(nil)
	require.NoError(t, err)
	require.NotEmpty(t, id)
	require.NotEmpty(t, auditInfo)

	ai := &AuditInfo{}
	require.NoError(t, ai.FromBytes(auditInfo))
	require.NotEmpty(t, ai.EnrollmentId)
	require.NotEmpty(t, ai.RevocationHandle)
}

func TestProvider_DeserializeVerifier_NonECDSA(t *testing.T) {
	t.Parallel()
	p, err := NewProvider("./testdata/msp", "", "apple", nil)
	require.NoError(t, err)

	// Create RSA key and serialize it
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pubPEM, err := PemEncodeKey(&rsaKey.PublicKey)
	require.NoError(t, err)

	sID := &msp2.SerializedIdentity{Mspid: "apple", IdBytes: pubPEM}
	raw, err := proto.Marshal(sID)
	require.NoError(t, err)

	_, err = p.DeserializeVerifier(raw)
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected *ecdsa.PublicKey")
}

func TestNewProviderWithBCCSPConfig_SWExplicit(t *testing.T) {
	t.Parallel()
	conf := &config.BCCSP{Default: "SW"}
	p, err := NewProviderWithBCCSPConfig("./testdata/msp", "", "apple", nil, conf)
	require.NoError(t, err)
	require.NotNil(t, p)
	require.False(t, p.IsRemote())
}
