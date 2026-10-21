/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/configtest"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/tlsconfig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
)

// TestReferenceConfiguration checks the logging and fsc sections of docs/configuration.md
// against the view platform code that reads them. tls: blocks decode into the tlsconfig
// types that tlsconfig.Resolve* decode them into; the resolvers themselves cannot be used,
// because they read the certificate files and the document's paths are placeholders.
func TestReferenceConfiguration(t *testing.T) {
	t.Parallel()
	r := configtest.Load(t)

	r.Read(
		"logging.format",
		"logging.spec",
		"logging.otel.sanitize",
		"fsc.id",
		"fsc.identity.cert.file",
		"fsc.identity.key.file",
		"fsc.client.certs",
		"fsc.kvs.cache.size",
		"fsc.metrics.provider",
	)

	r.Decode("fsc.tls", &tlsconfig.ServerTLS{})

	r.Read(
		"fsc.grpc.enabled",
		"fsc.grpc.address",
		"fsc.grpc.connectionTimeout",
		"fsc.grpc.maxRecvMsgSize",
		"fsc.grpc.maxSendMsgSize",
		"fsc.grpc.authentication.timeWindow",
	)
	r.Decode("fsc.grpc.tls", &tlsconfig.ServerTLS{})
	r.Decode("fsc.grpc.keepalive", &grpc.ServerKeepAliveConfig{})

	r.Read(
		"fsc.p2p.type",
		"fsc.p2p.listenAddress",
		"fsc.p2p.incomingMessagesBufferSize",
		"fsc.p2p.streamReaderBufferSize",
		"fsc.p2p.maxRecvMsgSize",
		"fsc.p2p.maxSendMsgSize",
		"fsc.p2p.maxRespondersPerPeer",
		"fsc.p2p.opts.libp2p.bootstrapNode",
		"fsc.p2p.opts.libp2p.connManager.lowWater",
		"fsc.p2p.opts.libp2p.connManager.highWater",
		"fsc.p2p.opts.libp2p.connManager.gracePeriod",
		"fsc.p2p.opts.websocket.maxSubConns",
		"fsc.p2p.opts.websocket.corsAllowedOrigins",
	)
	r.Decode("fsc.p2p.opts.websocket.tls", &tlsconfig.TLS{})

	persistences, ok := r.RawSubtree("fsc.persistences")
	require.True(t, ok, "fsc.persistences is not documented")
	for name := range persistences {
		key := "fsc.persistences." + name
		r.Read(key + ".type")
		switch typ := r.GetString(key + ".type"); typ {
		case "memory":
			// Takes no options: documented opts stay uncovered and fail RequireComplete.
		case "sqlite":
			r.Decode(key+".opts", &sqlite.Config{})
		case "postgres":
			r.Decode(key+".opts", &postgres.Config{})
		default:
			t.Errorf("%s.type: unknown persistence type [%s]", key, typ)
		}
	}

	r.Read("fsc.web.enabled", "fsc.web.address")

	r.Read("fsc.view.remote.timeout")
	r.Decode("fsc.web.tls", &tlsconfig.ServerTLS{})

	r.Decode("fsc.tracing", &tracing.Config{}, "otlp.tls")
	r.Decode("fsc.tracing.otlp.tls", &tlsconfig.ClientTLS{})

	// Decoded into a type unexported from platform/view/sdk/dig/support/endpoint, whose
	// TestReferenceResolvers checks it.
	r.Delegate("fsc.endpoint.resolvers")

	r.RequireComplete("logging", "fsc")
}
