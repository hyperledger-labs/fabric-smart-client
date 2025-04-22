/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabricdev

import (
	fdevdriver "github.com/hyperledger-labs/fabric-smart-client/docs/fabric/fabricdev/core/fabricdev/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	mspdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	vdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/multiplexed"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/dig"
)

func NewDriver(in struct {
	dig.In

	EnvelopeKVS         driver.EnvelopeStore
	MetadataKVS         driver.MetadataStore
	EndorseTxKVS        driver.EndorseTxStore
	ConfigProvider      config.Provider
	MetricsProvider     metrics.Provider
	EndpointService     vdriver.EndpointService
	SigService          *sig.Service
	DeserializerManager mspdriver.DeserializerManager
	IdProvider          vdriver.IdentityProvider
	KVS                 *kvs.KVS
	Publisher           events.Publisher
	Hasher              hash.Hasher
	TracerProvider      trace.TracerProvider
	Drivers             multiplexed.Driver
}) core.NamedDriver {
	d := core.NamedDriver{
		Name: "fabricdev",
		Driver: fdevdriver.NewProvider(
			in.EnvelopeKVS,
			in.MetadataKVS,
			in.EndorseTxKVS,
			in.ConfigProvider,
			in.MetricsProvider,
			in.EndpointService,
			in.SigService,
			in.DeserializerManager,
			in.IdProvider,
			in.KVS,
			in.Publisher,
			in.Hasher,
			in.TracerProvider,
			in.Drivers,
		),
	}
	return d
}
