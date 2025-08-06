/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	vdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id/kms"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id/kms/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/multiplexed"
	kvs2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/dig"
)

func newTracerProvider(metricsProvider metrics.Provider, configService vdriver.ConfigService) (TracerProviders, error) {
	base, err := tracing.NewProviderFromConfigService(configService)
	if err != nil {
		return TracerProviders{}, err
	}
	backed := tracing.NewProviderWithBackingProvider(base, metricsProvider)
	return TracerProviders{
		Base:    base,
		Backed:  backed,
		Default: backed,
	}, nil
}

type TracerProviders struct {
	dig.Out

	// Base explicitly requires no bound metrics provider
	Base trace.TracerProvider `name:"base-tracer-provider"`

	// Backed explicitly requires a bound metrics provider
	Backed trace.TracerProvider `name:"backed-tracer-provider"`

	// Default is the default tracing provider
	Default tracing.Provider
}

func newMultiplexedDriver(in struct {
	dig.In
	Config  vdriver.ConfigService
	Drivers []dbdriver.NamedDriver `group:"db-drivers"`
}) multiplexed.Driver {
	return multiplexed.NewDriver(in.Config, in.Drivers...)
}

func newKVS(config vdriver.ConfigService, driver multiplexed.Driver) (*kvs2.KVS, error) {
	size, err := kvs2.CacheSizeFromConfig(config)
	if err != nil {
		return nil, err
	}

	return kvs2.New(utils.MustGet(kvs2.NewKeyValueStore(config, driver)), "_default", size)
}

func newKMSDriver(in struct {
	dig.In
	Config  vdriver.ConfigService
	Drivers []driver2.NamedDriver `group:"kms-drivers"`
}) (*kms.KMS, error) {
	driverName := utils.DefaultString(in.Config.GetString("fsc.identity.type"), "file")
	for _, driver := range in.Drivers {
		if string(driver.Name) == driverName {
			return &kms.KMS{Driver: driver.Driver}, nil
		}
	}
	return nil, errors.New("driver not found")
}
