/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package id

import (
	"strings"
	"testing"
	"time"

	idemix2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/idemix"
	x5092 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/x509"
	sig2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/core/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/badger"
	_ "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	registry2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/registry"
	msp2 "github.com/hyperledger/fabric/msp"
	"github.com/stretchr/testify/assert"
)

// TODO replace fakeProv some autogenerate mock like we use in platform/fabric/core/generic/msp/mock/config_provider
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

func TestInfoIdemix(t *testing.T) {
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

	id, _, err := p.Identity(nil)
	assert.NoError(t, err)

	s := Info(id, nil)
	assert.True(t, strings.HasPrefix(s, "MSP.Idemix: []"))
	assert.True(t, strings.HasSuffix(s, "[idemix][idemixorg.example.com][ADMIN]"))
}

func TestInfoX509(t *testing.T) {
	p, err := x5092.NewProvider("./testdata/x509", "apple", nil)
	assert.NoError(t, err)
	id, _, err := p.Identity(nil)
	assert.NoError(t, err)

	s := Info(id, nil)
	assert.Equal(t, "MSP.x509: [f+hVlmGaPejN2G0XDcESSMX2ol29WPcPQ+Fp3lOARBQ=][apple][auditor.org1.example.com]", s)
}
