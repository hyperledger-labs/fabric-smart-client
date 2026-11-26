/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix_test

import (
	"encoding/json"
	"strings"
	"testing"

	bccsp "github.com/IBM/idemix/bccsp/types"
	idemixconfig "github.com/IBM/idemix/msp/config"
	math "github.com/IBM/mathlib"
	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/driver/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/idemix"
	fabricmsp "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
)

func TestProvider(t *testing.T) { //nolint:paralleltest // idemix key import races inside fabric-amcl
	kvss, err := kvs.New(newKVS(t), "", kvs.DefaultCacheSize)
	require.NoError(t, err)

	sigService := sig.NewService(sig.NewMultiplexDeserializer(), newAuditInfo(t), newSignerInfo(t))

	config, err := fabricmsp.GetLocalMspConfigWithType("./testdata/idemix", nil, "idemix", "idemix")
	require.NoError(t, err)

	p, err := idemix.NewProviderWithEidRhNymPolicy(config, kvss, sigService)
	require.NoError(t, err)
	require.NotNil(t, p)

	p, err = idemix.NewProviderWithSigType(config, kvss, sigService, bccsp.Standard)
	require.NoError(t, err)
	require.NotNil(t, p)

	p, err = idemix.NewProviderWithSigType(config, kvss, sigService, bccsp.EidNymRhNym)
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

func TestIdentityWithEidRhNymPolicy(t *testing.T) { //nolint:paralleltest // idemix key import races inside fabric-amcl
	kvss, err := kvs.New(newKVS(t), "", kvs.DefaultCacheSize)
	require.NoError(t, err)

	sigService := sig.NewService(sig.NewMultiplexDeserializer(), newAuditInfo(t), newSignerInfo(t))

	config, err := fabricmsp.GetLocalMspConfigWithType("./testdata/idemix", nil, "idemix", "idemix")
	require.NoError(t, err)

	p, err := idemix.NewProviderWithEidRhNymPolicy(config, kvss, sigService)
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

	p, err = idemix.NewProviderWithAnyPolicy(config, kvss, sigService)
	require.NoError(t, err)
	require.NotNil(t, p)

	id, audit, err = p.Identity(&fdriver.IdentityOptions{EIDExtension: true})
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

func TestIdentityStandard(t *testing.T) { //nolint:paralleltest // idemix key import races inside fabric-amcl
	kvss, err := kvs.New(newKVS(t), "", kvs.DefaultCacheSize)
	require.NoError(t, err)

	sigService := sig.NewService(sig.NewMultiplexDeserializer(), newAuditInfo(t), newSignerInfo(t))

	config, err := fabricmsp.GetLocalMspConfigWithType("./testdata/idemix", nil, "idemix", "idemix")
	require.NoError(t, err)

	p, err := idemix.NewProviderWithSigType(config, kvss, sigService, bccsp.Standard)
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

	p, err = idemix.NewProviderWithAnyPolicy(config, kvss, sigService)
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

func TestAuditWithEidRhNymPolicy(t *testing.T) { //nolint:paralleltest // idemix key import races inside fabric-amcl
	kvss, err := kvs.New(newKVS(t), "", kvs.DefaultCacheSize)
	require.NoError(t, err)
	sigService := sig.NewService(sig.NewMultiplexDeserializer(), newAuditInfo(t), newSignerInfo(t))

	config, err := fabricmsp.GetLocalMspConfigWithType("./testdata/idemix", nil, "idemix", "idemix")
	require.NoError(t, err)
	p, err := idemix.NewProviderWithEidRhNymPolicy(config, kvss, sigService)
	require.NoError(t, err)
	require.NotNil(t, p)

	config, err = fabricmsp.GetLocalMspConfigWithType("./testdata/idemix2", nil, "idemix", "idemix")
	require.NoError(t, err)
	p2, err := idemix.NewProviderWithEidRhNymPolicy(config, kvss, sigService)
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

func TestProvider_DeserializeSigner(t *testing.T) { //nolint:paralleltest // idemix key import races inside fabric-amcl
	kvss, err := kvs.New(newKVS(t), "", kvs.DefaultCacheSize)
	require.NoError(t, err)

	sigService := sig.NewService(sig.NewMultiplexDeserializer(), newAuditInfo(t), newSignerInfo(t))

	config, err := fabricmsp.GetLocalMspConfigWithType("./testdata/sameissuer/idemix", nil, "idemix", "idemix")
	require.NoError(t, err)
	p, err := idemix.NewProviderWithEidRhNymPolicy(config, kvss, sigService)
	require.NoError(t, err)
	require.NotNil(t, p)

	config, err = fabricmsp.GetLocalMspConfigWithType("./testdata/sameissuer/idemix2", nil, "idemix", "idemix")
	require.NoError(t, err)
	p2, err := idemix.NewProviderWithEidRhNymPolicy(config, kvss, sigService)
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

func TestIdentityFromFabricCA(t *testing.T) { //nolint:paralleltest // idemix key import races inside fabric-amcl
	kvss, err := kvs.New(newKVS(t), "", kvs.DefaultCacheSize)
	require.NoError(t, err)
	sigService := sig.NewService(sig.NewMultiplexDeserializer(), newAuditInfo(t), newSignerInfo(t))

	config, err := idemix.GetLocalMspConfigWithType("./testdata/charlie.ExtraId2", "charlie.ExtraId2")
	require.NoError(t, err)

	p, err := idemix.NewProviderWithSigTypeAncCurve(config, kvss, sigService, bccsp.Standard, math.BN254)
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

	p, err = idemix.NewProviderWithSigTypeAncCurve(config, kvss, sigService, bccsp.Standard, math.BN254)
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

	p, err = idemix.NewProviderWithSigTypeAncCurve(config, kvss, sigService, idemix.Any, math.BN254)
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

func TestIdentityFromFabricCAWithEidRhNymPolicy(t *testing.T) { //nolint:paralleltest // idemix key import races inside fabric-amcl
	kvss, err := kvs.New(newKVS(t), "", kvs.DefaultCacheSize)
	require.NoError(t, err)
	sigService := sig.NewService(sig.NewMultiplexDeserializer(), newAuditInfo(t), newSignerInfo(t))

	config, err := idemix.GetLocalMspConfigWithType("./testdata/charlie.ExtraId2", "charlie.ExtraId2")
	require.NoError(t, err)

	p, err := idemix.NewProviderWithSigTypeAncCurve(config, kvss, sigService, bccsp.EidNymRhNym, math.BN254)
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

	p, err = idemix.NewProviderWithAnyPolicyAndCurve(config, kvss, sigService, math.BN254)
	require.NoError(t, err)
	require.NotNil(t, p)

	id, audit, err = p.Identity(&fdriver.IdentityOptions{EIDExtension: true})
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

func TestProvider_IdentityManagerMethods(t *testing.T) { //nolint:paralleltest // idemix key import races inside fabric-amcl
	kvss, err := kvs.New(newKVS(t), "", kvs.DefaultCacheSize)
	require.NoError(t, err)

	sigService := sig.NewService(sig.NewMultiplexDeserializer(), newAuditInfo(t), newSignerInfo(t))

	config, err := fabricmsp.GetLocalMspConfigWithType("./testdata/idemix", nil, "idemix", "idemix")
	require.NoError(t, err)

	p, err := idemix.NewProviderWithEidRhNymPolicy(config, kvss, sigService)
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

// A pseudonym deserializes only under the MSP ID of its provider: relabelled
// with another MSP ID it is refused, although its credential is valid.
func TestProvider_DeserializeRefusesOtherMSPID(t *testing.T) { //nolint:paralleltest // idemix key import races inside fabric-amcl
	kvss, err := kvs.New(newKVS(t), "", kvs.DefaultCacheSize)
	require.NoError(t, err)
	config, err := fabricmsp.GetLocalMspConfigWithType("./testdata/idemix", nil, "idemix", "idemix")
	require.NoError(t, err)
	p, err := idemix.NewProviderWithAnyPolicy(config, kvss, sig.NewService(sig.NewMultiplexDeserializer(), newAuditInfo(t), newSignerInfo(t)))
	require.NoError(t, err)

	id, _, err := p.Identity(nil)
	require.NoError(t, err)
	_, err = p.DeserializeVerifier(id)
	require.NoError(t, err)

	si := &msp.SerializedIdentity{}
	require.NoError(t, proto.Unmarshal(id, si))
	si.Mspid = "Org1MSP"
	relabelled, err := proto.Marshal(si)
	require.NoError(t, err)
	_, err = p.DeserializeVerifier(relabelled)
	require.ErrorContains(t, err, "expected MSP ID [idemix], got [Org1MSP]")
	_, err = p.DeserializeSigner(relabelled)
	require.Error(t, err)
}

func TestNewProvider_Errors(t *testing.T) { //nolint:paralleltest // idemix key import races inside fabric-amcl
	kvss, err := kvs.New(newKVS(t), "", kvs.DefaultCacheSize)
	require.NoError(t, err)
	sigService := sig.NewService(sig.NewMultiplexDeserializer(), newAuditInfo(t), newSignerInfo(t))
	validConfig, err := fabricmsp.GetLocalMspConfigWithType("./testdata/idemix", nil, "idemix", "idemix")
	require.NoError(t, err)
	// testdata/idemix2 holds another user of the same issuer.
	otherConfig, err := fabricmsp.GetLocalMspConfigWithType("./testdata/idemix2", nil, "idemix", "idemix")
	require.NoError(t, err)

	t.Run("unsupported curve", func(t *testing.T) { //nolint:paralleltest // idemix key import races inside fabric-amcl
		_, err := idemix.NewProviderWithAnyPolicyAndCurve(validConfig, kvss, sigService, math.CurveID(3))
		require.ErrorContains(t, err, "unsupported curve ID")
		_, err = idemix.NewProviderWithSigTypeAncCurve(validConfig, kvss, sigService, bccsp.Standard, math.CurveID(3))
		require.ErrorContains(t, err, "unsupported curve ID")
	})

	t.Run("Any verifies best effort and signs standard", func(t *testing.T) { //nolint:paralleltest // idemix key import races inside fabric-amcl
		p, err := idemix.NewProviderWithAnyPolicy(validConfig, kvss, sigService)
		require.NoError(t, err)
		require.Equal(t, bccsp.BestEffort, p.VerType)
		_, audit, err := p.Identity(nil)
		require.NoError(t, err)
		require.Nil(t, audit, "standard signatures carry no audit info")
	})

	cryptoProvider, err := idemix.NewBCCSP(math.FP256BN_AMCL)
	require.NoError(t, err)

	var conf, otherConf idemixconfig.IdemixMSPConfig
	require.NoError(t, proto.UnmarshalV1(validConfig.Config, &conf))
	require.NoError(t, proto.UnmarshalV1(otherConfig.Config, &otherConf))

	// mutated returns an MSPConfig holding a copy of conf with mutate applied.
	mutated := func(mutate func(*idemixconfig.IdemixMSPConfig)) *msp.MSPConfig {
		c := proto.Clone(&conf).(*idemixconfig.IdemixMSPConfig)
		mutate(c)
		raw, err := proto.MarshalV1(c)
		require.NoError(t, err)
		return &msp.MSPConfig{Config: raw}
	}

	tests := []struct {
		name    string
		conf    *msp.MSPConfig
		sigType bccsp.SignatureType
		wantErr string
	}{
		{
			name:    "nil conf",
			conf:    nil,
			wantErr: "setup error: nil conf reference",
		},
		{
			name:    "unparseable config",
			conf:    &msp.MSPConfig{Config: []byte("invalid-proto")},
			wantErr: "failed unmarshalling idemix provider config",
		},
		{
			name:    "corrupted issuer public key",
			conf:    mutated(func(c *idemixconfig.IdemixMSPConfig) { c.Ipk = []byte("corrupted-ipk") }),
			wantErr: "failed to unmarshal issuer public key",
		},
		{
			name:    "corrupted revocation public key",
			conf:    mutated(func(c *idemixconfig.IdemixMSPConfig) { c.RevocationPk = []byte("corrupted-rev-pk") }),
			wantErr: "failed to import revocation public key",
		},
		{
			name:    "missing signer",
			conf:    mutated(func(c *idemixconfig.IdemixMSPConfig) { c.Signer = nil }),
			wantErr: "no signer information found",
		},
		{
			name: "corrupted signer secret key",
			conf: mutated(func(c *idemixconfig.IdemixMSPConfig) {
				c.Signer = &idemixconfig.IdemixMSPSignerConfig{Sk: []byte("corrupted-sk"), Cred: conf.Signer.Cred}
			}),
			wantErr: "failed importing signer secret key",
		},
		{
			name: "corrupted credential",
			conf: mutated(func(c *idemixconfig.IdemixMSPConfig) {
				c.Signer = &idemixconfig.IdemixMSPSignerConfig{Sk: conf.Signer.Sk, Cred: []byte("corrupted-cred")}
			}),
			wantErr: "credential is not cryptographically valid",
		},
		{
			name: "credential of another user",
			conf: mutated(func(c *idemixconfig.IdemixMSPConfig) {
				c.Signer = proto.Clone(otherConf.Signer).(*idemixconfig.IdemixMSPSignerConfig)
				c.Signer.Sk = conf.Signer.Sk
			}),
			wantErr: "credential is not cryptographically valid",
		},
		{
			name:    "unknown signature type",
			conf:    validConfig,
			sigType: bccsp.SignatureType(99),
			wantErr: "unknown verification type [99]",
		},
	}
	for _, tc := range tests { //nolint:paralleltest // idemix key import races inside fabric-amcl
		t.Run(tc.name, func(t *testing.T) {
			// The zero sigType is bccsp.Standard.
			p, err := idemix.NewProvider(tc.conf, sigService, tc.sigType, cryptoProvider)
			require.ErrorContains(t, err, tc.wantErr)
			require.Nil(t, p)
		})
	}
}

func TestProvider_Remote(t *testing.T) { //nolint:paralleltest // idemix key import races inside fabric-amcl
	sigService := sig.NewService(sig.NewMultiplexDeserializer(), newAuditInfo(t), newSignerInfo(t))
	validConfig, err := fabricmsp.GetLocalMspConfigWithType("./testdata/idemix", nil, "idemix", "idemix")
	require.NoError(t, err)

	cryptoProvider, err := idemix.NewBCCSP(math.FP256BN_AMCL)
	require.NoError(t, err)

	var conf idemixconfig.IdemixMSPConfig
	require.NoError(t, proto.UnmarshalV1(validConfig.Config, &conf))

	remoteConf := proto.Clone(&conf).(*idemixconfig.IdemixMSPConfig)
	remoteConf.Signer = &idemixconfig.IdemixMSPSignerConfig{
		EnrollmentId: "remote-user",
	}
	raw, err := proto.MarshalV1(remoteConf)
	require.NoError(t, err)

	pRemote, err := idemix.NewProvider(&msp.MSPConfig{Config: raw}, sigService, bccsp.Standard, cryptoProvider)
	require.NoError(t, err)
	require.True(t, pRemote.IsRemote())

	_, _, err = pRemote.Identity(nil)
	require.ErrorContains(t, err, "failed deriving nym")
}

func TestProvider_Identity_AuditInfoAndMockSigner(t *testing.T) { //nolint:paralleltest // idemix key import races inside fabric-amcl
	kvss, err := kvs.New(newKVS(t), "", kvs.DefaultCacheSize)
	require.NoError(t, err)
	sigService := sig.NewService(sig.NewMultiplexDeserializer(), newAuditInfo(t), newSignerInfo(t))
	config, err := fabricmsp.GetLocalMspConfigWithType("./testdata/idemix", nil, "idemix", "idemix")
	require.NoError(t, err)

	p, err := idemix.NewProviderWithEidRhNymPolicy(config, kvss, sigService)
	require.NoError(t, err)

	// Identity with invalid audit info
	_, _, err = p.Identity(&fdriver.IdentityOptions{AuditInfo: []byte("garbage")})
	require.ErrorContains(t, err, "failed deserializing audit info")

	// Identity with valid audit info re-use
	id1, audit1, err := p.Identity(nil)
	require.NoError(t, err)
	require.NotNil(t, id1)
	require.NotNil(t, audit1)

	id2, audit2, err := p.Identity(&fdriver.IdentityOptions{AuditInfo: audit1, EIDExtension: true})
	require.NoError(t, err)
	require.NotNil(t, id2)
	require.NotNil(t, audit2)

	ai1, err := p.DeserializeAuditInfo(audit1)
	require.NoError(t, err)
	require.NoError(t, ai1.Match(id2))

	// Mock SignerService registration failure
	mockSigService := &mock.SignerService{}
	mockSigService.RegisterSignerReturns(errors.New("mock register failure"))
	pWithMock, err := idemix.NewProviderWithSigType(config, kvss, mockSigService, bccsp.Standard)
	require.NoError(t, err)

	_, _, err = pWithMock.Identity(nil)
	require.ErrorContains(t, err, "mock register failure")
}

func TestProvider_DeserializeErrors(t *testing.T) { //nolint:paralleltest // idemix key import races inside fabric-amcl
	kvss, err := kvs.New(newKVS(t), "", kvs.DefaultCacheSize)
	require.NoError(t, err)
	sigService := sig.NewService(sig.NewMultiplexDeserializer(), newAuditInfo(t), newSignerInfo(t))
	config, err := fabricmsp.GetLocalMspConfigWithType("./testdata/idemix", nil, "idemix", "idemix")
	require.NoError(t, err)

	p, err := idemix.NewProviderWithEidRhNymPolicy(config, kvss, sigService)
	require.NoError(t, err)

	validID, _, err := p.Identity(nil)
	require.NoError(t, err)

	for _, tc := range malformedIdentities(t, validID) { //nolint:paralleltest // idemix key import races inside fabric-amcl
		t.Run("DeserializeSigningIdentity rejects "+tc.name, func(t *testing.T) {
			_, err := p.DeserializeSigningIdentity(tc.raw)
			require.ErrorContains(t, err, tc.wantErr)
		})
		t.Run("DeserializeVerifier rejects "+tc.name, func(t *testing.T) {
			_, err := p.DeserializeVerifier(tc.raw)
			require.ErrorContains(t, err, tc.wantErr)
		})
	}

	// Provider.Info error paths
	_, err = p.Info([]byte("garbage"), nil)
	require.ErrorContains(t, err, "failed to unmarshal to msp.SerializedIdentity{}")

	_, err = p.Info(validID, []byte("garbage"))
	var syntaxErr *json.SyntaxError
	require.ErrorAs(t, err, &syntaxErr)

	_, auditDiff, err := p.Identity(nil)
	require.NoError(t, err)
	_, err = p.Info(validID, auditDiff)
	require.ErrorContains(t, err, "eid nym does not match")

	// DeserializeSigningIdentity with missing nym key in KVS
	kvssSame1, err := kvs.New(newKVS(t), "", kvs.DefaultCacheSize)
	require.NoError(t, err)

	// A separately named store: newKVS(t) twice would share one in-memory database.
	s2, err := mem.NewDriver().NewKVS("", "isolateddb")
	require.NoError(t, err)
	kvssSame2, err := kvs.New(s2, "", kvs.DefaultCacheSize)
	require.NoError(t, err)

	sameConfig, err := fabricmsp.GetLocalMspConfigWithType("./testdata/sameissuer/idemix", nil, "idemix", "idemix")
	require.NoError(t, err)

	pSame1, err := idemix.NewProviderWithAnyPolicy(sameConfig, kvssSame1, sigService)
	require.NoError(t, err)
	pSame2, err := idemix.NewProviderWithAnyPolicy(sameConfig, kvssSame2, sigService)
	require.NoError(t, err)

	idFrom2, _, err := pSame2.Identity(nil)
	require.NoError(t, err)

	// pSame2's identity has nym key stored in kvssSame2, not in kvssSame1 (pSame1's KVS)
	_, err = pSame1.DeserializeSigningIdentity(idFrom2)
	require.ErrorContains(t, err, "cannot find nym secret key")
	_, err = pSame1.DeserializeSigner(idFrom2)
	require.ErrorContains(t, err, "cannot find nym secret key")
}
