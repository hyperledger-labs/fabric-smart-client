/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/dig"

	vdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id/kms"
	kmsdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id/kms/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/multiplexed"
	sqlite2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
)

func newMuxDriver(p vdriver.ConfigService, drivers ...dbdriver.NamedDriver) multiplexed.Driver {
	return newMultiplexedDriver(struct {
		dig.In
		Config  vdriver.ConfigService
		Drivers []dbdriver.NamedDriver `group:"db-drivers"`
	}{
		Config:  p,
		Drivers: drivers,
	})
}

func newKMS(p vdriver.ConfigService, drivers ...kmsdriver.NamedDriver) (*kms.KMS, error) {
	return newKMSDriver(struct {
		dig.In
		Config  vdriver.ConfigService
		Drivers []kmsdriver.NamedDriver `group:"kms-drivers"`
	}{
		Config:  p,
		Drivers: drivers,
	})
}

type fakeKMSDriver struct {
	kmsdriver.Driver
}

func TestNewTracerProvider(t *testing.T) {
	t.Parallel()

	t.Run("success with default config", func(t *testing.T) {
		t.Parallel()
		p := providerFrom(t, "")
		tp, err := newTracerProvider(&disabled.Provider{}, p)
		require.NoError(t, err)
		assert.NotNil(t, tp.Base)
		assert.NotNil(t, tp.Backed)
		assert.NotNil(t, tp.Default)
	})

	t.Run("error with invalid tls config", func(t *testing.T) {
		t.Parallel()
		p := providerFrom(t, `
fsc:
  tracing:
    provider: otlp
    otlp:
      tls:
        enabled: true
        cert:
          file: non-existent-cert.crt
`)
		_, err := newTracerProvider(&disabled.Provider{}, p)
		require.Error(t, err)
	})
}

func TestNewMultiplexedDriver(t *testing.T) {
	t.Parallel()

	p := providerFrom(t, "")
	namedDriver := mem.NewNamedDriver(sqlite2.NewDbProvider())
	d := newMuxDriver(p, namedDriver)
	assert.NotNil(t, d)
}

func TestNewKVS(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		p := providerFrom(t, `
fsc:
  kvs:
    cache:
      size: 100
`)
		namedDriver := mem.NewNamedDriver(sqlite2.NewDbProvider())
		muxDriver := newMuxDriver(p, namedDriver)

		kvsInst, err := newKVS(p, muxDriver)
		require.NoError(t, err)
		assert.NotNil(t, kvsInst)
	})

	t.Run("invalid cache size error", func(t *testing.T) {
		t.Parallel()
		p := providerFrom(t, `
fsc:
  kvs:
    cache:
      size: -5
`)
		namedDriver := mem.NewNamedDriver(sqlite2.NewDbProvider())
		muxDriver := newMuxDriver(p, namedDriver)

		_, err := newKVS(p, muxDriver)
		require.ErrorContains(t, err, "invalid cache size configuration")
	})
}

func TestNewKMSDriver(t *testing.T) {
	t.Parallel()

	t.Run("default to file driver success", func(t *testing.T) {
		t.Parallel()
		p := providerFrom(t, "")
		mockDriver := &fakeKMSDriver{}
		kmsInst, err := newKMS(p, kmsdriver.NamedDriver{Name: "file", Driver: mockDriver})
		require.NoError(t, err)
		require.NotNil(t, kmsInst)
		assert.Equal(t, mockDriver, kmsInst.Driver)
	})

	t.Run("custom driver success", func(t *testing.T) {
		t.Parallel()
		p := providerFrom(t, `
fsc:
  identity:
    type: vault
`)
		mockDriver := &fakeKMSDriver{}
		kmsInst, err := newKMS(p,
			kmsdriver.NamedDriver{Name: "file", Driver: &fakeKMSDriver{}},
			kmsdriver.NamedDriver{Name: "vault", Driver: mockDriver},
		)
		require.NoError(t, err)
		require.NotNil(t, kmsInst)
		assert.Equal(t, mockDriver, kmsInst.Driver)
	})

	t.Run("driver not found error", func(t *testing.T) {
		t.Parallel()
		p := providerFrom(t, `
fsc:
  identity:
    type: hsm
`)
		_, err := newKMS(p, kmsdriver.NamedDriver{Name: "file", Driver: &fakeKMSDriver{}})
		require.EqualError(t, err, "driver not found")
	})
}
