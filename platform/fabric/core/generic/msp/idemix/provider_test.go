/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix_test

import (
	"strings"
	"testing"

	bccsp "github.com/IBM/idemix/bccsp/types"
	math "github.com/IBM/mathlib"
	"github.com/stretchr/testify/require"

	idemix2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/idemix"
	fabricmsp "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
)

func TestProvider(t *testing.T) { //nolint:paralleltest
	kvss, err := kvs.New(newKVS(t), "", kvs.DefaultCacheSize)
	require.NoError(t, err)

	sigService := sig.NewService(sig.NewMultiplexDeserializer(), newAuditInfo(t), newSignerInfo(t))

	config, err := fabricmsp.GetLocalMspConfigWithType("./testdata/idemix", nil, "idemix", "idemix")
	require.NoError(t, err)

	p, err := idemix2.NewProviderWithEidRhNymPolicy(config, kvss, sigService)
	require.NoError(t, err)
	require.NotNil(t, p)

	p, err = idemix2.NewProviderWithSigType(config, kvss, sigService, bccsp.Standard)
	require.NoError(t, err)
	require.NotNil(t, p)

	p, err = idemix2.NewProviderWithSigType(config, kvss, sigService, bccsp.EidNymRhNym)
	require.NoError(t, err)
	require.NotNil(t, p)
}

func newSignerInfo(t *testing.T) driver.SignerInfoStore {
	t.Helper()
	s, err := mem.NewDriver().NewSignerInfo("")
	require.NoError(t, err)
	return s
}

func newAuditInfo(t *testing.T) driver.AuditInfoStore {
	t.Helper()
	s, err := mem.NewDriver().NewAuditInfo("")
	require.NoError(t, err)
	return s
}

func newKVS(t *testing.T) driver.KeyValueStore {
	t.Helper()
	s, err := mem.NewDriver().NewKVS("")
	require.NoError(t, err)
	return s
}

func TestIdentityWithEidRhNymPolicy(t *testing.T) { //nolint:paralleltest
	kvss, err := kvs.New(newKVS(t), "", kvs.DefaultCacheSize)
	require.NoError(t, err)

	sigService := sig.NewService(sig.NewMultiplexDeserializer(), newAuditInfo(t), newSignerInfo(t))

	config, err := fabricmsp.GetLocalMspConfigWithType("./testdata/idemix", nil, "idemix", "idemix")
	require.NoError(t, err)

	p, err := idemix2.NewProviderWithEidRhNymPolicy(config, kvss, sigService)
	require.NoError(t, err)
	require.NotNil(t, p)

	id, audit, err := p.Identity(nil)
	require.NoError(t, err)
	require.NotNil(t, id)
	require.NotNil(t, audit)
	info, err := p.Info(id, audit)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(info, "MSP.Idemix: [alice]"))
	require.True(t, strings.HasSuffix(info, "[idemix][idemixorg.example.com][ADMIN]"))

	auditInfo, err := p.DeserializeAuditInfo(audit)
	require.NoError(t, err)
	require.NoError(t, auditInfo.Match(id))

	signer, err := p.DeserializeSigner(id)
	require.NoError(t, err)
	verifier, err := p.DeserializeVerifier(id)
	require.NoError(t, err)

	sigma, err := signer.Sign([]byte("hello world!!!"))
	require.NoError(t, err)
	require.NoError(t, verifier.Verify([]byte("hello world!!!"), sigma))

	p, err = idemix2.NewProviderWithAnyPolicy(config, kvss, sigService)
	require.NoError(t, err)
	require.NotNil(t, p)

	id, audit, err = p.Identity(&driver2.IdentityOptions{EIDExtension: true})
	require.NoError(t, err)
	require.NotNil(t, id)
	require.NotNil(t, audit)
	info, err = p.Info(id, audit)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(info, "MSP.Idemix: [alice]"))
	require.True(t, strings.HasSuffix(info, "[idemix][idemixorg.example.com][ADMIN]"))

	auditInfo, err = p.DeserializeAuditInfo(audit)
	require.NoError(t, err)
	require.NoError(t, auditInfo.Match(id))

	signer, err = p.DeserializeSigner(id)
	require.NoError(t, err)
	verifier, err = p.DeserializeVerifier(id)
	require.NoError(t, err)

	sigma, err = signer.Sign([]byte("hello world!!!"))
	require.NoError(t, err)
	require.NoError(t, verifier.Verify([]byte("hello world!!!"), sigma))
}

func TestIdentityStandard(t *testing.T) { //nolint:paralleltest
	kvss, err := kvs.New(newKVS(t), "", kvs.DefaultCacheSize)
	require.NoError(t, err)

	sigService := sig.NewService(sig.NewMultiplexDeserializer(), newAuditInfo(t), newSignerInfo(t))

	config, err := fabricmsp.GetLocalMspConfigWithType("./testdata/idemix", nil, "idemix", "idemix")
	require.NoError(t, err)

	p, err := idemix2.NewProviderWithSigType(config, kvss, sigService, bccsp.Standard)
	require.NoError(t, err)
	require.NotNil(t, p)

	id, audit, err := p.Identity(nil)
	require.NoError(t, err)
	require.NotNil(t, id)
	require.Nil(t, audit)

	signer, err := p.DeserializeSigner(id)
	require.NoError(t, err)
	verifier, err := p.DeserializeVerifier(id)
	require.NoError(t, err)

	sigma, err := signer.Sign([]byte("hello world!!!"))
	require.NoError(t, err)
	require.NoError(t, verifier.Verify([]byte("hello world!!!"), sigma))

	p, err = idemix2.NewProviderWithStandardPolicy(config, kvss, sigService)
	require.NoError(t, err)
	require.NotNil(t, p)

	id, audit, err = p.Identity(nil)
	require.NoError(t, err)
	require.NotNil(t, id)
	require.Nil(t, audit)

	signer, err = p.DeserializeSigner(id)
	require.NoError(t, err)
	verifier, err = p.DeserializeVerifier(id)
	require.NoError(t, err)

	sigma, err = signer.Sign([]byte("hello world!!!"))
	require.NoError(t, err)
	require.NoError(t, verifier.Verify([]byte("hello world!!!"), sigma))

	p, err = idemix2.NewProviderWithAnyPolicy(config, kvss, sigService)
	require.NoError(t, err)
	require.NotNil(t, p)

	id, audit, err = p.Identity(nil)
	require.NoError(t, err)
	require.NotNil(t, id)
	require.Nil(t, audit)

	signer, err = p.DeserializeSigner(id)
	require.NoError(t, err)
	verifier, err = p.DeserializeVerifier(id)
	require.NoError(t, err)

	sigma, err = signer.Sign([]byte("hello world!!!"))
	require.NoError(t, err)
	require.NoError(t, verifier.Verify([]byte("hello world!!!"), sigma))
}

func TestAuditWithEidRhNymPolicy(t *testing.T) { //nolint:paralleltest
	kvss, err := kvs.New(newKVS(t), "", kvs.DefaultCacheSize)
	require.NoError(t, err)
	sigService := sig.NewService(sig.NewMultiplexDeserializer(), newAuditInfo(t), newSignerInfo(t))

	config, err := fabricmsp.GetLocalMspConfigWithType("./testdata/idemix", nil, "idemix", "idemix")
	require.NoError(t, err)
	p, err := idemix2.NewProviderWithEidRhNymPolicy(config, kvss, sigService)
	require.NoError(t, err)
	require.NotNil(t, p)

	config, err = fabricmsp.GetLocalMspConfigWithType("./testdata/idemix2", nil, "idemix", "idemix")
	require.NoError(t, err)
	p2, err := idemix2.NewProviderWithEidRhNymPolicy(config, kvss, sigService)
	require.NoError(t, err)
	require.NotNil(t, p2)

	id, audit, err := p.Identity(nil)
	require.NoError(t, err)
	require.NotNil(t, id)
	require.NotNil(t, audit)
	id2, audit2, err := p2.Identity(nil)
	require.NoError(t, err)
	require.NotNil(t, id2)
	require.NotNil(t, audit2)

	auditInfo, err := p.DeserializeAuditInfo(audit)
	require.NoError(t, err)
	require.NoError(t, auditInfo.Match(id))
	require.Error(t, auditInfo.Match(id2))

	auditInfo, err = p2.DeserializeAuditInfo(audit)
	require.NoError(t, err)
	require.NoError(t, auditInfo.FromBytes(audit2))
	require.NoError(t, auditInfo.Match(id2))
	require.Error(t, auditInfo.Match(id))
}

func TestProvider_DeserializeSigner(t *testing.T) { //nolint:paralleltest
	kvss, err := kvs.New(newKVS(t), "", kvs.DefaultCacheSize)
	require.NoError(t, err)

	sigService := sig.NewService(sig.NewMultiplexDeserializer(), newAuditInfo(t), newSignerInfo(t))

	config, err := fabricmsp.GetLocalMspConfigWithType("./testdata/sameissuer/idemix", nil, "idemix", "idemix")
	require.NoError(t, err)
	p, err := idemix2.NewProviderWithEidRhNymPolicy(config, kvss, sigService)
	require.NoError(t, err)
	require.NotNil(t, p)

	config, err = fabricmsp.GetLocalMspConfigWithType("./testdata/sameissuer/idemix2", nil, "idemix", "idemix")
	require.NoError(t, err)
	p2, err := idemix2.NewProviderWithEidRhNymPolicy(config, kvss, sigService)
	require.NoError(t, err)
	require.NotNil(t, p2)

	id, _, err := p.Identity(nil)
	require.NoError(t, err)

	id2, _, err := p2.Identity(nil)
	require.NoError(t, err)

	// This must work
	signer, err := p.DeserializeSigner(id)
	require.NoError(t, err)
	verifier, err := p.DeserializeVerifier(id)
	require.NoError(t, err)
	msg := []byte("Hello World!!!")
	sigma, err := signer.Sign(msg)
	require.NoError(t, err)
	require.NoError(t, verifier.Verify(msg, sigma))

	// Try to deserialize id2 with provider for id, must fail
	_, err = p.DeserializeSigner(id2)
	require.Error(t, err)
	_, err = p.DeserializeVerifier(id2)
	require.NoError(t, err)

	// this must work
	des := sig.NewMultiplexDeserializer()
	des.AddDeserializer(p)
	des.AddDeserializer(p2)
	signer, err = des.DeserializeSigner(id)
	require.NoError(t, err)
	verifier, err = des.DeserializeVerifier(id)
	require.NoError(t, err)
	sigma, err = signer.Sign(msg)
	require.NoError(t, err)
	require.NoError(t, verifier.Verify(msg, sigma))
}

// TestProvider_VerifyRejectsMismatchedNym ensures that an association proof cannot be
// paired with a Nym public key other than the one it was generated for. Without binding
// the proof to a specific Nym, an adversary could take a valid proof from one identity
// and attach an arbitrary attacker-chosen Nym to it.
func TestProvider_VerifyRejectsMismatchedNym(t *testing.T) { //nolint:paralleltest
	kvss, err := kvs.New(newKVS(), "", kvs.DefaultCacheSize)
	require.NoError(t, err)

	sigService := sig.NewService(sig.NewMultiplexDeserializer(), newAuditInfo(), newSignerInfo())

	config, err := fabricmsp.GetLocalMspConfigWithType("./testdata/sameissuer/idemix", nil, "idemix", "idemix")
	require.NoError(t, err)
	p, err := idemix2.NewProviderWithEidRhNymPolicy(config, kvss, sigService)
	require.NoError(t, err)
	require.NotNil(t, p)

	config, err = fabricmsp.GetLocalMspConfigWithType("./testdata/sameissuer/idemix2", nil, "idemix", "idemix")
	require.NoError(t, err)
	p2, err := idemix2.NewProviderWithEidRhNymPolicy(config, kvss, sigService)
	require.NoError(t, err)
	require.NotNil(t, p2)

	id, _, err := p.Identity(nil)
	require.NoError(t, err)
	id2, _, err := p2.Identity(nil)
	require.NoError(t, err)

	r, err := p.Deserialize(id, false)
	require.NoError(t, err)
	r2, err := p.Deserialize(id2, false)
	require.NoError(t, err)

	// Forge an identity that carries id's association proof but id2's Nym public key.
	forged := *r.Identity
	forged.NymPublicKey = r2.NymPublicKey
	require.Error(t, forged.Validate())
}

func TestIdentityFromFabricCA(t *testing.T) { //nolint:paralleltest
	kvss, err := kvs.New(newKVS(t), "", kvs.DefaultCacheSize)
	require.NoError(t, err)
	sigService := sig.NewService(sig.NewMultiplexDeserializer(), newAuditInfo(t), newSignerInfo(t))

	config, err := idemix2.GetLocalMspConfigWithType("./testdata/charlie.ExtraId2", "charlie.ExtraId2")
	require.NoError(t, err)

	p, err := idemix2.NewProviderWithSigTypeAncCurve(config, kvss, sigService, bccsp.Standard, math.BN254)
	require.NoError(t, err)
	require.NotNil(t, p)

	id, audit, err := p.Identity(nil)
	require.NoError(t, err)
	require.NotNil(t, id)
	require.Nil(t, audit)

	signer, err := p.DeserializeSigner(id)
	require.NoError(t, err)
	verifier, err := p.DeserializeVerifier(id)
	require.NoError(t, err)

	sigma, err := signer.Sign([]byte("hello world!!!"))
	require.NoError(t, err)
	require.NoError(t, verifier.Verify([]byte("hello world!!!"), sigma))

	p, err = idemix2.NewProviderWithSigTypeAncCurve(config, kvss, sigService, bccsp.Standard, math.BN254)
	require.NoError(t, err)
	require.NotNil(t, p)

	id, audit, err = p.Identity(nil)
	require.NoError(t, err)
	require.NotNil(t, id)
	require.Nil(t, audit)

	signer, err = p.DeserializeSigner(id)
	require.NoError(t, err)
	verifier, err = p.DeserializeVerifier(id)
	require.NoError(t, err)

	sigma, err = signer.Sign([]byte("hello world!!!"))
	require.NoError(t, err)
	require.NoError(t, verifier.Verify([]byte("hello world!!!"), sigma))

	p, err = idemix2.NewProviderWithSigTypeAncCurve(config, kvss, sigService, idemix2.Any, math.BN254)
	require.NoError(t, err)
	require.NotNil(t, p)

	id, audit, err = p.Identity(nil)
	require.NoError(t, err)
	require.NotNil(t, id)
	require.Nil(t, audit)

	signer, err = p.DeserializeSigner(id)
	require.NoError(t, err)
	verifier, err = p.DeserializeVerifier(id)
	require.NoError(t, err)

	sigma, err = signer.Sign([]byte("hello world!!!"))
	require.NoError(t, err)
	require.NoError(t, verifier.Verify([]byte("hello world!!!"), sigma))
}

func TestIdentityFromFabricCAWithEidRhNymPolicy(t *testing.T) { //nolint:paralleltest
	kvss, err := kvs.New(newKVS(t), "", kvs.DefaultCacheSize)
	require.NoError(t, err)
	sigService := sig.NewService(sig.NewMultiplexDeserializer(), newAuditInfo(t), newSignerInfo(t))

	config, err := idemix2.GetLocalMspConfigWithType("./testdata/charlie.ExtraId2", "charlie.ExtraId2")
	require.NoError(t, err)

	p, err := idemix2.NewProviderWithSigTypeAncCurve(config, kvss, sigService, bccsp.EidNymRhNym, math.BN254)
	require.NoError(t, err)
	require.NotNil(t, p)

	// get an identity with its own audit info from the provider
	// id is in its serialized form
	id, audit, err := p.Identity(nil)
	require.NoError(t, err)
	require.NotNil(t, id)
	require.NotNil(t, audit)
	info, err := p.Info(id, audit)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(info, "MSP.Idemix: [charlie.ExtraId2]"))
	require.True(t, strings.HasSuffix(info, "[charlie.ExtraId2][][MEMBER]"))

	auditInfo, err := p.DeserializeAuditInfo(audit)
	require.NoError(t, err)
	require.NoError(t, auditInfo.Match(id))

	signer, err := p.DeserializeSigner(id)
	require.NoError(t, err)
	verifier, err := p.DeserializeVerifier(id)
	require.NoError(t, err)

	sigma, err := signer.Sign([]byte("hello world!!!"))
	require.NoError(t, err)
	require.NoError(t, verifier.Verify([]byte("hello world!!!"), sigma))

	p, err = idemix2.NewProviderWithAnyPolicyAndCurve(config, kvss, sigService, math.BN254)
	require.NoError(t, err)
	require.NotNil(t, p)

	id, audit, err = p.Identity(&driver2.IdentityOptions{EIDExtension: true})
	require.NoError(t, err)
	require.NotNil(t, id)
	require.NotNil(t, audit)
	info, err = p.Info(id, audit)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(info, "MSP.Idemix: [charlie.ExtraId2]"))
	require.True(t, strings.HasSuffix(info, "[charlie.ExtraId2][][MEMBER]"))

	auditInfo, err = p.DeserializeAuditInfo(audit)
	require.NoError(t, err)
	require.NoError(t, auditInfo.Match(id))

	signer, err = p.DeserializeSigner(id)
	require.NoError(t, err)
	verifier, err = p.DeserializeVerifier(id)
	require.NoError(t, err)

	sigma, err = signer.Sign([]byte("hello world!!!"))
	require.NoError(t, err)
	require.NoError(t, verifier.Verify([]byte("hello world!!!"), sigma))
}

func TestProvider_IdentityManagerMethods(t *testing.T) { //nolint:paralleltest
	kvss, err := kvs.New(newKVS(t), "", kvs.DefaultCacheSize)
	require.NoError(t, err)

	sigService := sig.NewService(sig.NewMultiplexDeserializer(), newAuditInfo(t), newSignerInfo(t))

	config, err := fabricmsp.GetLocalMspConfigWithType("./testdata/idemix", nil, "idemix", "idemix")
	require.NoError(t, err)

	p, err := idemix2.NewProviderWithEidRhNymPolicy(config, kvss, sigService)
	require.NoError(t, err)
	require.NotNil(t, p)

	require.False(t, p.IsRemote())
	require.NotEmpty(t, p.String())

	id, _, err := p.Identity(nil)
	require.NoError(t, err)

	sigID, err := p.DeserializeSigningIdentity(id)
	require.NoError(t, err)
	require.NotNil(t, sigID)

	_, err = p.DeserializeVerifier(id)
	require.NoError(t, err)

	info, err := p.Info(id, nil)
	require.NoError(t, err)
	require.NotEmpty(t, info)
}
