/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix_test

import (
	"strings"
	"testing"
	"time"

	bccsp "github.com/IBM/idemix/bccsp/schemes"
	idemix2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/idemix"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	sig2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/core/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/badger"
	_ "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	registry2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/registry"
	msp2 "github.com/hyperledger/fabric/msp"
	"github.com/stretchr/testify/assert"
)

// TODO replace fakeProv this with msp/mock/config_provider
type fakeProv struct {
	typ  string
	path string
}

func (f *fakeProv) GetString(key string) string {
	return f.typ
}

func (f *fakeProv) GetInt(key string) int {
	return 0
}

func (f *fakeProv) GetDuration(key string) time.Duration {
	return time.Duration(0)
}

func (f *fakeProv) GetBool(key string) bool {
	return false
}

func (f *fakeProv) GetStringSlice(key string) []string {
	return nil
}

func (f *fakeProv) IsSet(key string) bool {
	return false
}

func (f *fakeProv) UnmarshalKey(key string, rawVal interface{}) error {
	*(rawVal.(*badger.Opts)) = badger.Opts{
		Path: f.path,
	}

	return nil
}

func (f *fakeProv) ConfigFileUsed() string {
	return ""
}

func (f *fakeProv) GetPath(key string) string {
	return ""
}

func (f *fakeProv) TranslatePath(path string) string {
	return ""
}

func TestProvider(t *testing.T) {
	registry := registry2.New()
	registry.RegisterService(&fakeProv{typ: "memory"})

	kvss, err := kvs.New(registry, "memory", "")
	assert.NoError(t, err)
	assert.NoError(t, registry.RegisterService(kvss))
	sigService := sig2.NewSignService(registry, nil, kvss)
	assert.NoError(t, registry.RegisterService(sigService))

	config, err := msp2.GetLocalMspConfigWithType("./testdata/idemix", nil, "idemix", "idemix")
	assert.NoError(t, err)

	p, err := idemix2.NewEIDNymProvider(config, registry)
	assert.NoError(t, err)
	assert.NotNil(t, p)

	p, err = idemix2.NewProviderWithSigType(config, registry, bccsp.Standard)
	assert.NoError(t, err)
	assert.NotNil(t, p)

	p, err = idemix2.NewProviderWithSigType(config, registry, bccsp.EidNym)
	assert.NoError(t, err)
	assert.NotNil(t, p)
}

func TestIdentityEidNym(t *testing.T) {
	registry := registry2.New()
	registry.RegisterService(&fakeProv{typ: "memory"})

	kvss, err := kvs.New(registry, "memory", "")
	assert.NoError(t, err)
	assert.NoError(t, registry.RegisterService(kvss))
	sigService := sig2.NewSignService(registry, nil, kvss)
	assert.NoError(t, registry.RegisterService(sigService))

	config, err := msp2.GetLocalMspConfigWithType("./testdata/idemix", nil, "idemix", "idemix")
	assert.NoError(t, err)

	p, err := idemix2.NewEIDNymProvider(config, registry)
	assert.NoError(t, err)
	assert.NotNil(t, p)

	id, audit, err := p.Identity(nil)
	assert.NoError(t, err)
	assert.NotNil(t, id)
	assert.NotNil(t, audit)
	info, err := p.Info(id, audit)
	assert.NoError(t, err)
	assert.True(t, strings.HasPrefix(info, "MSP.Idemix: [idemix]"))
	assert.True(t, strings.HasSuffix(info, "[idemix][idemixorg.example.com][ADMIN]"))

	auditInfo, err := p.DeserializeAuditInfo(audit)
	assert.NoError(t, err)
	assert.NoError(t, auditInfo.Match(id))

	signer, err := p.DeserializeSigner(id)
	assert.NoError(t, err)
	verifier, err := p.DeserializeVerifier(id)
	assert.NoError(t, err)

	sigma, err := signer.Sign([]byte("hello world!!!"))
	assert.NoError(t, err)
	assert.NoError(t, verifier.Verify([]byte("hello world!!!"), sigma))

	p, err = idemix2.NewAnyProvider(config, registry)
	assert.NoError(t, err)
	assert.NotNil(t, p)

	id, audit, err = p.Identity(&driver2.IdentityOptions{EIDExtension: true})
	assert.NoError(t, err)
	assert.NotNil(t, id)
	assert.NotNil(t, audit)
	info, err = p.Info(id, audit)
	assert.NoError(t, err)
	assert.True(t, strings.HasPrefix(info, "MSP.Idemix: [idemix]"))
	assert.True(t, strings.HasSuffix(info, "[idemix][idemixorg.example.com][ADMIN]"))

	auditInfo, err = p.DeserializeAuditInfo(audit)
	assert.NoError(t, err)
	assert.NoError(t, auditInfo.Match(id))

	signer, err = p.DeserializeSigner(id)
	assert.NoError(t, err)
	verifier, err = p.DeserializeVerifier(id)
	assert.NoError(t, err)

	sigma, err = signer.Sign([]byte("hello world!!!"))
	assert.NoError(t, err)
	assert.NoError(t, verifier.Verify([]byte("hello world!!!"), sigma))
}

func TestIdentityStandard(t *testing.T) {
	registry := registry2.New()
	registry.RegisterService(&fakeProv{typ: "memory"})

	kvss, err := kvs.New(registry, "memory", "")
	assert.NoError(t, err)
	assert.NoError(t, registry.RegisterService(kvss))
	sigService := sig2.NewSignService(registry, nil, kvss)
	assert.NoError(t, registry.RegisterService(sigService))

	config, err := msp2.GetLocalMspConfigWithType("./testdata/idemix", nil, "idemix", "idemix")
	assert.NoError(t, err)

	p, err := idemix2.NewProviderWithSigType(config, registry, bccsp.Standard)
	assert.NoError(t, err)
	assert.NotNil(t, p)

	id, audit, err := p.Identity(nil)
	assert.NoError(t, err)
	assert.NotNil(t, id)
	assert.Nil(t, audit)

	signer, err := p.DeserializeSigner(id)
	assert.NoError(t, err)
	verifier, err := p.DeserializeVerifier(id)
	assert.NoError(t, err)

	sigma, err := signer.Sign([]byte("hello world!!!"))
	assert.NoError(t, err)
	assert.NoError(t, verifier.Verify([]byte("hello world!!!"), sigma))

	p, err = idemix2.NewStandardProvider(config, registry)
	assert.NoError(t, err)
	assert.NotNil(t, p)

	id, audit, err = p.Identity(nil)
	assert.NoError(t, err)
	assert.NotNil(t, id)
	assert.Nil(t, audit)

	signer, err = p.DeserializeSigner(id)
	assert.NoError(t, err)
	verifier, err = p.DeserializeVerifier(id)
	assert.NoError(t, err)

	sigma, err = signer.Sign([]byte("hello world!!!"))
	assert.NoError(t, err)
	assert.NoError(t, verifier.Verify([]byte("hello world!!!"), sigma))

	p, err = idemix2.NewAnyProvider(config, registry)
	assert.NoError(t, err)
	assert.NotNil(t, p)

	id, audit, err = p.Identity(nil)
	assert.NoError(t, err)
	assert.NotNil(t, id)
	assert.Nil(t, audit)

	signer, err = p.DeserializeSigner(id)
	assert.NoError(t, err)
	verifier, err = p.DeserializeVerifier(id)
	assert.NoError(t, err)

	sigma, err = signer.Sign([]byte("hello world!!!"))
	assert.NoError(t, err)
	assert.NoError(t, verifier.Verify([]byte("hello world!!!"), sigma))
}

func TestAuditEidNym(t *testing.T) {
	registry := registry2.New()
	registry.RegisterService(&fakeProv{typ: "memory"})

	kvss, err := kvs.New(registry, "memory", "")
	assert.NoError(t, err)
	assert.NoError(t, registry.RegisterService(kvss))
	sigService := sig2.NewSignService(registry, nil, kvss)
	assert.NoError(t, registry.RegisterService(sigService))

	config, err := msp2.GetLocalMspConfigWithType("./testdata/idemix", nil, "idemix", "idemix")
	assert.NoError(t, err)
	p, err := idemix2.NewEIDNymProvider(config, registry)
	assert.NoError(t, err)
	assert.NotNil(t, p)

	config, err = msp2.GetLocalMspConfigWithType("./testdata/idemix2", nil, "idemix", "idemix")
	assert.NoError(t, err)
	p2, err := idemix2.NewEIDNymProvider(config, registry)
	assert.NoError(t, err)
	assert.NotNil(t, p2)

	id, audit, err := p.Identity(nil)
	assert.NoError(t, err)
	assert.NotNil(t, id)
	assert.NotNil(t, audit)

	id2, audit2, err := p2.Identity(nil)
	assert.NoError(t, err)
	assert.NotNil(t, id2)
	assert.NotNil(t, audit2)

	auditInfo, err := p.DeserializeAuditInfo(audit)
	assert.NoError(t, err)
	assert.NoError(t, auditInfo.Match(id))
	assert.Error(t, auditInfo.Match(id2))

	auditInfo, err = p2.DeserializeAuditInfo(audit)
	assert.NoError(t, err)
	assert.NoError(t, auditInfo.FromBytes(audit2))
	assert.NoError(t, auditInfo.Match(id2))
	assert.Error(t, auditInfo.Match(id))
}

func TestProvider_DeserializeSigner(t *testing.T) {
	registry := registry2.New()
	registry.RegisterService(&fakeProv{typ: "memory"})

	kvss, err := kvs.New(registry, "memory", "")
	assert.NoError(t, err)
	assert.NoError(t, registry.RegisterService(kvss))
	sigService := sig2.NewSignService(registry, nil, kvss)
	assert.NoError(t, registry.RegisterService(sigService))

	config, err := msp2.GetLocalMspConfigWithType("./testdata/sameissuer/idemix", nil, "idemix", "idemix")
	assert.NoError(t, err)
	p, err := idemix2.NewEIDNymProvider(config, registry)
	assert.NoError(t, err)
	assert.NotNil(t, p)

	config, err = msp2.GetLocalMspConfigWithType("./testdata/sameissuer/idemix2", nil, "idemix", "idemix")
	assert.NoError(t, err)
	p2, err := idemix2.NewEIDNymProvider(config, registry)
	assert.NoError(t, err)
	assert.NotNil(t, p2)

	id, _, err := p.Identity(nil)
	assert.NoError(t, err)

	id2, _, err := p2.Identity(nil)
	assert.NoError(t, err)

	// This must work
	signer, err := p.DeserializeSigner(id)
	assert.NoError(t, err)
	verifier, err := p.DeserializeVerifier(id)
	assert.NoError(t, err)
	msg := []byte("Hello World!!!")
	sigma, err := signer.Sign(msg)
	assert.NoError(t, err)
	assert.NoError(t, verifier.Verify(msg, sigma))

	// Try to deserialize id2 with provider for id, must fail
	signer, err = p.DeserializeSigner(id2)
	assert.Error(t, err)
	verifier, err = p.DeserializeVerifier(id2)
	assert.NoError(t, err)

	// this must work
	des, err := sig2.NewMultiplexDeserializer(registry)
	assert.NoError(t, err)
	des.AddDeserializer(p)
	des.AddDeserializer(p2)
	signer, err = des.DeserializeSigner(id)
	assert.NoError(t, err)
	verifier, err = des.DeserializeVerifier(id)
	assert.NoError(t, err)
	sigma, err = signer.Sign(msg)
	assert.NoError(t, err)
	assert.NoError(t, verifier.Verify(msg, sigma))
}
