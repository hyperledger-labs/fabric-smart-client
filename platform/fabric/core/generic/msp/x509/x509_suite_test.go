/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package x509

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	msp2 "github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	mspdrivermock "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/driver/mock"
)

// ----- helpers -----

func generateSelfSignedCert(t *testing.T) (*ecdsa.PrivateKey, []byte) {
	t.Helper()
	sk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-user"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &sk.PublicKey, sk)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	return sk, certPEM
}

func serializeIdentity(t *testing.T, mspID string, certPEM []byte) []byte {
	t.Helper()
	sID := &msp2.SerializedIdentity{Mspid: mspID, IdBytes: certPEM}
	raw, err := proto.Marshal(sID)
	require.NoError(t, err)
	return raw
}

// ----- ecdsa.go tests -----

func TestNewSigner(t *testing.T) {
	t.Parallel()
	id, signer, verifier, err := NewSigner()
	require.NoError(t, err)
	require.NotNil(t, id)
	require.NotNil(t, signer)
	require.NotNil(t, verifier)

	msg := []byte("hello world")
	sig, err := signer.Sign(msg)
	require.NoError(t, err)
	require.NoError(t, verifier.Verify(msg, sig))
	require.Error(t, verifier.Verify([]byte("tampered"), sig))
}

func TestPemEncodeDecodeKey(t *testing.T) {
	t.Parallel()
	sk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	// public key round-trip
	pubPEM, err := PemEncodeKey(sk.Public())
	require.NoError(t, err)
	decoded, err := PemDecodeKey(pubPEM)
	require.NoError(t, err)
	require.IsType(t, &ecdsa.PublicKey{}, decoded)

	// private key round-trip
	privPEM, err := PemEncodeKey(sk)
	require.NoError(t, err)
	decoded, err = PemDecodeKey(privPEM)
	require.NoError(t, err)
	require.IsType(t, &ecdsa.PrivateKey{}, decoded)
}

func TestPemDecodeKey_Errors(t *testing.T) {
	t.Parallel()
	_, err := PemDecodeKey([]byte("not pem"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "not PEM encoded")

	badBlock := pem.EncodeToMemory(&pem.Block{Type: "UNKNOWN", Bytes: []byte("bad")})
	_, err = PemDecodeKey(badBlock)
	require.Error(t, err)
	require.Contains(t, err.Error(), "bad key type")
}

func TestPemDecodeKey_Certificate(t *testing.T) {
	t.Parallel()
	_, certPEM := generateSelfSignedCert(t)
	key, err := PemDecodeKey(certPEM)
	require.NoError(t, err)
	require.IsType(t, &ecdsa.PublicKey{}, key)
}

func TestNewIdentityFromBytes(t *testing.T) {
	t.Parallel()
	id, signer, _, err := NewSigner()
	require.NoError(t, err)

	identity, verifier, err := NewIdentityFromBytes(id)
	require.NoError(t, err)
	require.NotNil(t, identity)

	msg := []byte("test message")
	sig, err := signer.Sign(msg)
	require.NoError(t, err)
	require.NoError(t, verifier.Verify(msg, sig))
}

func TestNewIdentityFromBytes_Errors(t *testing.T) {
	t.Parallel()
	_, _, err := NewIdentityFromBytes([]byte("garbage"))
	require.Error(t, err)
}

func TestIsLowS(t *testing.T) {
	t.Parallel()
	sk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	halfOrder := new(big.Int).Rsh(sk.PublicKey.Curve.Params().N, 1)

	low, err := IsLowS(&sk.PublicKey, big.NewInt(1))
	require.NoError(t, err)
	require.True(t, low)

	high := new(big.Int).Add(halfOrder, big.NewInt(1))
	low, err = IsLowS(&sk.PublicKey, high)
	require.NoError(t, err)
	require.False(t, low)
}

func TestToLowS(t *testing.T) {
	t.Parallel()
	sk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	halfOrder := new(big.Int).Rsh(sk.PublicKey.Curve.Params().N, 1)
	high := new(big.Int).Add(halfOrder, big.NewInt(1))

	result, modified, err := ToLowS(&sk.PublicKey, high)
	require.NoError(t, err)
	require.True(t, modified)

	low, err := IsLowS(&sk.PublicKey, result)
	require.NoError(t, err)
	require.True(t, low)
}

// ----- identity.go tests -----

func TestSerialize(t *testing.T) {
	t.Parallel()
	certPath := filepath.Join("testdata", "msp", "signcerts", "auditor.org1.example.com-cert.pem")
	raw, err := Serialize("apple", certPath)
	require.NoError(t, err)
	require.NotEmpty(t, raw)

	si := &msp2.SerializedIdentity{}
	require.NoError(t, proto.Unmarshal(raw, si))
	require.Equal(t, "apple", si.Mspid)
}

func TestSerialize_InvalidPath(t *testing.T) {
	t.Parallel()
	_, err := Serialize("apple", "/nonexistent/path/cert.pem")
	require.Error(t, err)
}

func TestSerializeRaw(t *testing.T) {
	t.Parallel()
	_, certPEM := generateSelfSignedCert(t)
	raw, err := SerializeRaw("testmsp", certPEM)
	require.NoError(t, err)
	require.NotEmpty(t, raw)
}

func TestSerializeRaw_InvalidPEM(t *testing.T) {
	t.Parallel()
	_, err := SerializeRaw("testmsp", []byte("not a cert"))
	require.Error(t, err)
}

func TestSerializeRaw_NilBytes(t *testing.T) {
	t.Parallel()
	_, err := SerializeRaw("testmsp", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil idBytes")
}

func TestGetCertFromPem_Errors(t *testing.T) {
	t.Parallel()
	_, err := getCertFromPem(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil idBytes")

	_, err = getCertFromPem([]byte("not pem"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "could not decode pem bytes")

	badCert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("bad der")})
	_, err = getCertFromPem(badCert)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to parse x509 cert")
}

// ----- cert.go tests -----

func TestPemDecodeCert(t *testing.T) {
	t.Parallel()
	_, certPEM := generateSelfSignedCert(t)
	cert, err := PemDecodeCert(certPEM)
	require.NoError(t, err)
	require.Equal(t, "test-user", cert.Subject.CommonName)
}

func TestPemDecodeCert_NotPEM(t *testing.T) {
	t.Parallel()
	_, err := PemDecodeCert([]byte("garbage"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "not PEM encoded")
}

func TestPemDecodeCert_BadType(t *testing.T) {
	t.Parallel()
	block := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: []byte("data")})
	_, err := PemDecodeCert(block)
	require.Error(t, err)
	require.Contains(t, err.Error(), "bad type")
}

// ----- audit.go tests -----

func TestAuditInfo_RoundTrip(t *testing.T) {
	t.Parallel()
	ai := &AuditInfo{
		EnrollmentId:     "user1",
		RevocationHandle: []byte("handle123"),
	}
	raw, err := ai.Bytes()
	require.NoError(t, err)

	ai2 := &AuditInfo{}
	require.NoError(t, ai2.FromBytes(raw))
	require.Equal(t, ai.EnrollmentId, ai2.EnrollmentId)
	require.Equal(t, ai.RevocationHandle, ai2.RevocationHandle)
}

func TestAuditInfo_FromBytes_Invalid(t *testing.T) {
	t.Parallel()
	ai := &AuditInfo{}
	require.Error(t, ai.FromBytes([]byte("not json")))
}

// ----- deserializer.go tests -----

func TestDeserializer_DeserializeVerifier(t *testing.T) {
	t.Parallel()
	id, signer, _, err := NewSigner()
	require.NoError(t, err)

	des := &Deserializer{}
	verifier, err := des.DeserializeVerifier(id)
	require.NoError(t, err)

	msg := []byte("payload")
	sig, err := signer.Sign(msg)
	require.NoError(t, err)
	require.NoError(t, verifier.Verify(msg, sig))
}

func TestDeserializer_DeserializeVerifier_Invalid(t *testing.T) {
	t.Parallel()
	des := &Deserializer{}
	_, err := des.DeserializeVerifier([]byte("garbage"))
	require.Error(t, err)
}

func TestDeserializer_DeserializeSigner(t *testing.T) {
	t.Parallel()
	des := &Deserializer{}
	_, err := des.DeserializeSigner(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not supported")
}

func TestDeserializer_Info_WithTestdata(t *testing.T) {
	t.Parallel()
	certPEM, err := os.ReadFile(filepath.Join("testdata", "msp", "signcerts", "auditor.org1.example.com-cert.pem"))
	require.NoError(t, err)

	raw := serializeIdentity(t, "apple", certPEM)
	des := &Deserializer{}
	info, err := des.Info(raw, nil)
	require.NoError(t, err)
	require.Contains(t, info, "MSP.x509:")
	require.Contains(t, info, "apple")
	require.Contains(t, info, "auditor.org1.example.com")
}

func TestDeserializer_Info_Invalid(t *testing.T) {
	t.Parallel()
	des := &Deserializer{}
	_, err := des.Info([]byte("garbage"), nil)
	require.Error(t, err)
}

func TestDeserializer_String(t *testing.T) {
	t.Parallel()
	des := &Deserializer{}
	require.Equal(t, "Generic X509 Verifier Deserializer", des.String())
}

// ----- provider.go tests -----

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

// ----- setup.go tests -----

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
	sID := &msp2.SerializedIdentity{Mspid: "msp1", IdBytes: []byte("not pem")}
	raw, err := proto.Marshal(sID)
	require.NoError(t, err)

	_, err = GetEnrollmentID(raw)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not PEM encoded")
}

func TestGetEnrollmentID_BadBlockType(t *testing.T) {
	t.Parallel()
	block := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("data")})
	sID := &msp2.SerializedIdentity{Mspid: "msp1", IdBytes: block}
	raw, err := proto.Marshal(sID)
	require.NoError(t, err)

	_, err = GetEnrollmentID(raw)
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
	sID := &msp2.SerializedIdentity{Mspid: "msp1", IdBytes: []byte("not pem")}
	raw, err := proto.Marshal(sID)
	require.NoError(t, err)

	_, err = GetRevocationHandle(raw)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not PEM encoded")
}

func TestGetRevocationHandle_BadBlockType(t *testing.T) {
	t.Parallel()
	block := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("data")})
	sID := &msp2.SerializedIdentity{Mspid: "msp1", IdBytes: block}
	raw, err := proto.Marshal(sID)
	require.NoError(t, err)

	_, err = GetRevocationHandle(raw)
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
	raw, err := SerializeFromMSP("apple", "./testdata/msp")
	require.NoError(t, err)
	require.NotEmpty(t, raw)
}

func TestSerializeFromMSP_InvalidPath(t *testing.T) {
	t.Parallel()
	_, err := SerializeFromMSP("apple", "/nonexistent")
	require.Error(t, err)
}

// ----- config.go tests -----

func TestToPKCS11OptsOpts(t *testing.T) {
	t.Parallel()
	input := &config.PKCS11{
		Security: 256,
		Hash:     "SHA2",
		Library:  "/usr/lib/pkcs11.so",
		Label:    "token1",
		Pin:      "1234",
		KeyIDs: []config.KeyIDMapping{
			{SKI: "abc123", ID: "key1"},
		},
	}
	result := ToPKCS11OptsOpts(input)
	require.Equal(t, 256, result.Security)
	require.Equal(t, "SHA2", result.Hash)
	require.Equal(t, "/usr/lib/pkcs11.so", result.Library)
	require.Len(t, result.KeyIDs, 1)
	require.Equal(t, "abc123", result.KeyIDs[0].SKI)
}

// ----- loader.go tests -----

func TestToBCCSPOpts(t *testing.T) {
	t.Parallel()
	input := map[string]interface{}{
		"Default": "SW",
	}
	opts, err := ToBCCSPOpts(input)
	require.NoError(t, err)
	require.Equal(t, "SW", opts.Default)
}

func TestToBCCSPOpts_Invalid(t *testing.T) {
	t.Parallel()
	// Pass something that cannot be decoded
	opts, err := ToBCCSPOpts("not a map")
	require.Error(t, err)
	require.NotNil(t, opts)
}

// ----- setup.go: skiMapper tests -----

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

// ----- additional edge case tests -----

func TestPemEncodeKey_UnsupportedType(t *testing.T) {
	t.Parallel()
	_, err := PemEncodeKey("not a key")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unexpected key type")
}

func TestVerifier_InvalidSignature(t *testing.T) {
	t.Parallel()
	sk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	v := NewVerifier(&sk.PublicKey)
	err = v.Verify([]byte("msg"), []byte("not asn1"))
	require.Error(t, err)
}

func TestVerifier_WrongMessage(t *testing.T) {
	t.Parallel()
	id, signer, _, err := NewSigner()
	require.NoError(t, err)

	_, verifier, err := NewIdentityFromBytes(id)
	require.NoError(t, err)

	sig, err := signer.Sign([]byte("original"))
	require.NoError(t, err)
	err = verifier.Verify([]byte("different"), sig)
	require.Error(t, err)
	require.Contains(t, err.Error(), "signature not valid")
}

func TestGetEnrollmentID_InvalidCert(t *testing.T) {
	t.Parallel()
	badCert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("bad der")})
	sID := &msp2.SerializedIdentity{Mspid: "msp1", IdBytes: badCert}
	raw, err := proto.Marshal(sID)
	require.NoError(t, err)

	_, err = GetEnrollmentID(raw)
	require.Error(t, err)
	require.Contains(t, err.Error(), "pem bytes are not cert encoded")
}

func TestGetRevocationHandle_InvalidCert(t *testing.T) {
	t.Parallel()
	badCert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("bad der")})
	sID := &msp2.SerializedIdentity{Mspid: "msp1", IdBytes: badCert}
	raw, err := proto.Marshal(sID)
	require.NoError(t, err)

	_, err = GetRevocationHandle(raw)
	require.Error(t, err)
	require.Contains(t, err.Error(), "pem bytes are not cert encoded")
}

func TestPemDecodeCert_InvalidDER(t *testing.T) {
	t.Parallel()
	badCert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("bad der data")})
	_, err := PemDecodeCert(badCert)
	require.Error(t, err)
	require.Contains(t, err.Error(), "pem bytes are not cert encoded")
}

func TestPemDecodeKey_InvalidPrivateKey(t *testing.T) {
	t.Parallel()
	badKey := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("bad data")})
	_, err := PemDecodeKey(badKey)
	require.Error(t, err)
	require.Contains(t, err.Error(), "PKCS8 encoded")
}

func TestPemDecodeKey_InvalidPublicKey(t *testing.T) {
	t.Parallel()
	badKey := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte("bad data")})
	_, err := PemDecodeKey(badKey)
	require.Error(t, err)
	require.Contains(t, err.Error(), "PKIX encoded")
}

func TestDeserializer_DeserializeVerifier_BadKey(t *testing.T) {
	t.Parallel()
	// SerializedIdentity with non-ECDSA PEM
	badKey := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte("bad")})
	sID := &msp2.SerializedIdentity{Mspid: "msp1", IdBytes: badKey}
	raw, err := proto.Marshal(sID)
	require.NoError(t, err)

	des := &Deserializer{}
	_, err = des.DeserializeVerifier(raw)
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

func TestReadPemFile_InvalidPEMContent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f := filepath.Join(dir, "bad.pem")
	require.NoError(t, os.WriteFile(f, []byte("not pem content"), 0o644))
	_, err := readPemFile(f)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no pem content")
}

func TestReadFile_NonExistent(t *testing.T) {
	t.Parallel()
	_, err := readFile("/nonexistent/file.txt")
	require.Error(t, err)
}

func TestDeserializer_Info_BadCertPEM(t *testing.T) {
	t.Parallel()
	badCert := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("bad")})
	sID := &msp2.SerializedIdentity{Mspid: "msp1", IdBytes: badCert}
	raw, err := proto.Marshal(sID)
	require.NoError(t, err)

	des := &Deserializer{}
	_, err = des.Info(raw, nil)
	require.Error(t, err)
}

func TestGetPemMaterialFromDir_SkipsNonPEM(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notpem.txt"), []byte("plain text"), 0o644))
	certs, err := getPemMaterialFromDir(dir)
	require.NoError(t, err)
	require.Empty(t, certs)
}

func TestNewIdentityFromBytes_NonECDSA(t *testing.T) {
	t.Parallel()
	// Use a private key PEM which will decode but won't be *ecdsa.PublicKey
	badKey := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("bad")})
	sID := &msp2.SerializedIdentity{IdBytes: badKey}
	raw, err := proto.Marshal(sID)
	require.NoError(t, err)

	_, _, err = NewIdentityFromBytes(raw)
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

func TestGetSigningIdentity(t *testing.T) {
	t.Parallel()
	sID, err := GetSigningIdentity("./testdata/msp", "", "apple", nil)
	require.NoError(t, err)
	require.NotNil(t, sID)

	raw, err := sID.Serialize()
	require.NoError(t, err)
	require.NotEmpty(t, raw)
}

func TestGetSigningIdentity_InvalidPath(t *testing.T) {
	t.Parallel()
	_, err := GetSigningIdentity("/nonexistent", "", "msp1", nil)
	require.Error(t, err)
}

func TestLoadLocalMSPAt(t *testing.T) {
	t.Parallel()
	mspInst, err := LoadLocalMSPAt("./testdata/msp", "", "apple", BCCSPType, nil)
	require.NoError(t, err)
	require.NotNil(t, mspInst)
}

func TestLoadVerifyingMSPAt(t *testing.T) {
	t.Parallel()
	mspInst, err := LoadVerifyingMSPAt("./testdata/msp", "apple", BCCSPType)
	require.NoError(t, err)
	require.NotNil(t, mspInst)
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

func TestDeserializer_DeserializeVerifier_NonECDSA(t *testing.T) {
	t.Parallel()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pubPEM, err := PemEncodeKey(&rsaKey.PublicKey)
	require.NoError(t, err)

	sID := &msp2.SerializedIdentity{Mspid: "msp1", IdBytes: pubPEM}
	raw, err := proto.Marshal(sID)
	require.NoError(t, err)

	des := &Deserializer{}
	_, err = des.DeserializeVerifier(raw)
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected *ecdsa.PublicKey")
}

func TestPemEncodeKey_RSAPrivate(t *testing.T) {
	t.Parallel()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	privPEM, err := PemEncodeKey(rsaKey)
	require.NoError(t, err)
	require.NotEmpty(t, privPEM)
	require.Contains(t, string(privPEM), "PRIVATE KEY")
}

func TestPemEncodeKey_RSAPublic(t *testing.T) {
	t.Parallel()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pubPEM, err := PemEncodeKey(&rsaKey.PublicKey)
	require.NoError(t, err)
	require.NotEmpty(t, pubPEM)
	require.Contains(t, string(pubPEM), "PUBLIC KEY")
}

func TestDeserializer_Info_NonPEMIdBytes(t *testing.T) {
	t.Parallel()
	sID := &msp2.SerializedIdentity{Mspid: "msp1", IdBytes: []byte("not pem data")}
	raw, err := proto.Marshal(sID)
	require.NoError(t, err)

	des := &Deserializer{}
	_, err = des.Info(raw, nil)
	require.Error(t, err)
}

func TestReadPemFile_Success(t *testing.T) {
	t.Parallel()
	raw, err := readPemFile(filepath.Join("testdata", "msp", "signcerts", "auditor.org1.example.com-cert.pem"))
	require.NoError(t, err)
	require.NotEmpty(t, raw)
}

func TestNewProviderWithBCCSPConfig_SWExplicit(t *testing.T) {
	t.Parallel()
	conf := &config.BCCSP{Default: "SW"}
	p, err := NewProviderWithBCCSPConfig("./testdata/msp", "", "apple", nil, conf)
	require.NoError(t, err)
	require.NotNil(t, p)
	require.False(t, p.IsRemote())
}

// ----- loader.go tests -----

func TestIdentityLoader_Load(t *testing.T) {
	t.Parallel()
	mCfg := &mspdrivermock.Config{}
	mCfg.TranslatePathStub = func(path string) string { return path }
	mSignerSvc := &mspdrivermock.SignerService{}
	mMgr := &mspdrivermock.Manager{}
	mMgr.ConfigReturns(mCfg)
	mMgr.SignerServiceReturns(mSignerSvc)

	loader := &IdentityLoader{}
	err := loader.Load(mMgr, config.MSP{
		ID:    "test-id",
		MSPID: "apple",
		Path:  "./testdata/msp",
	})
	require.NoError(t, err)
	require.Equal(t, 1, mMgr.AddDeserializerCallCount())
	require.Equal(t, 1, mMgr.AddMSPCallCount())
	name, _, _, _ := mMgr.AddMSPArgsForCall(0)
	require.Equal(t, "test-id", name)
	require.Equal(t, 1, mMgr.SetDefaultIdentityCallCount())
}

func TestIdentityLoader_Load_WithMSPSubdir(t *testing.T) {
	t.Parallel()
	// Create a directory where the MSP config is at path/msp
	dir := t.TempDir()
	mspDir := filepath.Join(dir, "myid", "msp")

	// Copy testdata into mspDir
	for _, sub := range []string{"cacerts", "signcerts", "keystore", "admincerts"} {
		require.NoError(t, os.MkdirAll(filepath.Join(mspDir, sub), 0o755))
	}
	copyFile := func(src, dst string) {
		data, err := os.ReadFile(src)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(dst, data, 0o644))
	}
	copyFile(filepath.Join("testdata", "msp", "cacerts", "ca.org1.example.com-cert.pem"),
		filepath.Join(mspDir, "cacerts", "ca.pem"))
	copyFile(filepath.Join("testdata", "msp", "signcerts", "auditor.org1.example.com-cert.pem"),
		filepath.Join(mspDir, "signcerts", "cert.pem"))
	copyFile(filepath.Join("testdata", "msp", "keystore", "priv_sk"),
		filepath.Join(mspDir, "keystore", "priv_sk"))
	copyFile(filepath.Join("testdata", "msp", "signcerts", "auditor.org1.example.com-cert.pem"),
		filepath.Join(mspDir, "admincerts", "admin.pem"))

	mCfg := &mspdrivermock.Config{}
	mCfg.TranslatePathStub = func(path string) string { return path }
	mMgr := &mspdrivermock.Manager{}
	mMgr.ConfigReturns(mCfg)
	mMgr.SignerServiceReturns(&mspdrivermock.SignerService{})

	loader := &IdentityLoader{}
	err := loader.Load(mMgr, config.MSP{
		ID:    "myid",
		MSPID: "apple",
		Path:  filepath.Join(dir, "myid"),
	})
	require.NoError(t, err)
	require.Equal(t, 1, mMgr.AddMSPCallCount())
}

func TestIdentityLoader_Load_InvalidPath(t *testing.T) {
	t.Parallel()
	mCfg := &mspdrivermock.Config{}
	mCfg.TranslatePathStub = func(path string) string { return path }
	mMgr := &mspdrivermock.Manager{}
	mMgr.ConfigReturns(mCfg)

	loader := &IdentityLoader{}
	err := loader.Load(mMgr, config.MSP{
		ID:    "test-id",
		MSPID: "msp1",
		Path:  "/nonexistent/path",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to load BCCSP MSP configuration")
}

func TestIdentityLoader_Load_WithOpts(t *testing.T) {
	t.Parallel()
	mCfg := &mspdrivermock.Config{}
	mCfg.TranslatePathStub = func(path string) string { return path }
	mMgr := &mspdrivermock.Manager{}
	mMgr.ConfigReturns(mCfg)

	loader := &IdentityLoader{}
	err := loader.Load(mMgr, config.MSP{
		ID:    "test-id",
		MSPID: "apple",
		Path:  "./testdata/msp",
		Opts: map[interface{}]interface{}{
			"bccsp": map[string]interface{}{"Default": "SW"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, mMgr.AddMSPCallCount())
}

func TestIdentityLoader_Load_WithBadOpts(t *testing.T) {
	t.Parallel()
	mMgr := &mspdrivermock.Manager{}
	loader := &IdentityLoader{}
	err := loader.Load(mMgr, config.MSP{
		ID:    "test-id",
		MSPID: "apple",
		Path:  "ignored",
		Opts: map[interface{}]interface{}{
			"bccsp": "invalid-not-a-map",
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to unmarshal BCCSP opts")
}

func TestIdentityLoader_Load_AddMSPError(t *testing.T) {
	t.Parallel()
	mCfg := &mspdrivermock.Config{}
	mCfg.TranslatePathStub = func(path string) string { return path }
	mMgr := &mspdrivermock.Manager{}
	mMgr.ConfigReturns(mCfg)
	mMgr.AddMSPReturns(errors.New("mock AddMSP error"))

	loader := &IdentityLoader{}
	err := loader.Load(mMgr, config.MSP{
		ID:    "test-id",
		MSPID: "apple",
		Path:  "./testdata/msp",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed adding MSP")
}

func TestFolderIdentityLoader_Load(t *testing.T) {
	t.Parallel()
	// Create folder structure with one MSP subdirectory
	dir := t.TempDir()
	mspDir := filepath.Join(dir, "org1", "msp")

	for _, sub := range []string{"cacerts", "signcerts", "keystore", "admincerts"} {
		require.NoError(t, os.MkdirAll(filepath.Join(mspDir, sub), 0o755))
	}
	copyFile := func(src, dst string) {
		data, err := os.ReadFile(src)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(dst, data, 0o644))
	}
	copyFile(filepath.Join("testdata", "msp", "cacerts", "ca.org1.example.com-cert.pem"),
		filepath.Join(mspDir, "cacerts", "ca.pem"))
	copyFile(filepath.Join("testdata", "msp", "signcerts", "auditor.org1.example.com-cert.pem"),
		filepath.Join(mspDir, "signcerts", "cert.pem"))
	copyFile(filepath.Join("testdata", "msp", "keystore", "priv_sk"),
		filepath.Join(mspDir, "keystore", "priv_sk"))
	copyFile(filepath.Join("testdata", "msp", "signcerts", "auditor.org1.example.com-cert.pem"),
		filepath.Join(mspDir, "admincerts", "admin.pem"))

	mCfg := &mspdrivermock.Config{}
	mCfg.TranslatePathStub = func(path string) string { return path }
	mMgr := &mspdrivermock.Manager{}
	mMgr.ConfigReturns(mCfg)
	mMgr.SignerServiceReturns(&mspdrivermock.SignerService{})

	loader := &FolderIdentityLoader{IdentityLoader: &IdentityLoader{}}
	err := loader.Load(mMgr, config.MSP{
		Path: dir,
	})
	require.NoError(t, err)
	require.Equal(t, 1, mMgr.AddMSPCallCount())
	name, _, _, _ := mMgr.AddMSPArgsForCall(0)
	require.Equal(t, "org1", name)
}

func TestFolderIdentityLoader_Load_InvalidDir(t *testing.T) {
	t.Parallel()
	mCfg := &mspdrivermock.Config{}
	mCfg.TranslatePathStub = func(path string) string { return path }
	mMgr := &mspdrivermock.Manager{}
	mMgr.ConfigReturns(mCfg)

	loader := &FolderIdentityLoader{IdentityLoader: &IdentityLoader{}}
	err := loader.Load(mMgr, config.MSP{
		Path: "/nonexistent/dir",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed reading from")
}

func TestFolderIdentityLoader_Load_EmptyDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mCfg := &mspdrivermock.Config{}
	mCfg.TranslatePathStub = func(path string) string { return path }
	mMgr := &mspdrivermock.Manager{}
	mMgr.ConfigReturns(mCfg)

	loader := &FolderIdentityLoader{IdentityLoader: &IdentityLoader{}}
	err := loader.Load(mMgr, config.MSP{
		Path: dir,
	})
	require.NoError(t, err)
	require.Equal(t, 0, mMgr.AddMSPCallCount())
}

func TestFolderIdentityLoader_Load_SkipsFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notadir.txt"), []byte("data"), 0o644))

	mCfg := &mspdrivermock.Config{}
	mCfg.TranslatePathStub = func(path string) string { return path }
	mMgr := &mspdrivermock.Manager{}
	mMgr.ConfigReturns(mCfg)

	loader := &FolderIdentityLoader{IdentityLoader: &IdentityLoader{}}
	err := loader.Load(mMgr, config.MSP{
		Path: dir,
	})
	require.NoError(t, err)
	require.Equal(t, 0, mMgr.AddMSPCallCount())
}
