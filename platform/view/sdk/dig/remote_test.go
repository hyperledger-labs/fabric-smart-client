/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadRemoteConfig(t *testing.T) {
	t.Parallel()
	const (
		grpcOn  = "  grpc:\n    enabled: true\n    address: 127.0.0.1:0\n"
		grpcOff = "  grpc:\n    enabled: false\n"
		webOn   = "  web:\n    enabled: true\n    address: 127.0.0.1:0\n"
	)
	remote := func(body string) string { return "  view:\n    remote:\n" + body }
	for _, tc := range []struct {
		name    string
		yaml    string
		want    RemoteConfig
		wantErr []string
	}{
		{name: "absent, gRPC off", yaml: grpcOff, want: RemoteConfig{TimeWindow: DefaultCommandTimeWindow}},
		{name: "absent, gRPC on", yaml: grpcOn, wantErr: []string{"fsc.grpc.enabled", "fsc.view.remote.enabled", "backends: [grpc]"}},
		{name: "absent, web only", yaml: grpcOff + webOn, want: RemoteConfig{TimeWindow: DefaultCommandTimeWindow}},
		{name: "keys without enabled", yaml: grpcOff + remote("      backends: [grpc]\n"), wantErr: []string{"fsc.view.remote.enabled"}},
		{name: "timeout without enabled", yaml: grpcOff + remote("      timeout: 5m\n"), wantErr: []string{"fsc.view.remote.enabled"}},
		{name: "only enabled false", yaml: grpcOff + remote("      enabled: false\n"), want: RemoteConfig{TimeWindow: DefaultCommandTimeWindow}},
		{name: "keys with enabled false", yaml: grpcOff + remote("      enabled: false\n      backends: [grpc]\n"), wantErr: []string{"fsc.view.remote.enabled"}},
		{name: "enabled, no backends", yaml: grpcOn + remote("      enabled: true\n"), wantErr: []string{"fsc.view.remote.backends"}},
		{name: "grpc listed, listener off", yaml: grpcOff + remote("      enabled: true\n      backends: [grpc]\n"), wantErr: []string{"fsc.grpc.enabled"}},
		{name: "web listed, listener off", yaml: grpcOn + remote("      enabled: true\n      backends: [grpc, web]\n"), wantErr: []string{"fsc.web.enabled"}},
		{name: "duplicate", yaml: grpcOn + remote("      enabled: true\n      backends: [grpc, grpc]\n"), wantErr: []string{"fsc.view.remote.backends", "grpc"}},
		{name: "zero timeWindow", yaml: grpcOn + remote("      enabled: true\n      backends: [grpc]\n      timeWindow: 0s\n"), wantErr: []string{"fsc.view.remote.timeWindow"}},
		{name: "unparsable timeWindow", yaml: grpcOn + remote("      enabled: true\n      backends: [grpc]\n      timeWindow: soon\n"), wantErr: []string{"fsc.view.remote.timeWindow"}},
		{name: "unknown key with enabled", yaml: grpcOn + remote("      enabled: true\n      backends: [grpc]\n      timeWindows: 1s\n      maxMsgSize: 1\n"), wantErr: []string{"fsc.view.remote", "[maxmsgsize timewindows]"}},
		{name: "every known key with enabled", yaml: grpcOn + remote("      enabled: true\n      backends: [grpc]\n      timeout: 1m\n      timeWindow: 1m\n      clients: []\n      maxMessageSize: 1\n"), want: RemoteConfig{Enabled: true, TimeWindow: time.Minute, MaxMessageSize: 1, Backends: []string{"grpc"}}},
		{name: "negative maxMessageSize", yaml: grpcOn + remote("      enabled: true\n      backends: [grpc]\n      maxMessageSize: -1\n"), wantErr: []string{"fsc.view.remote.maxMessageSize"}},
		{
			name: "full",
			yaml: grpcOn + webOn + remote("      enabled: true\n      timeWindow: 5m\n      maxMessageSize: 1024\n      backends: [grpc, web]\n"),
			want: RemoteConfig{Enabled: true, TimeWindow: 5 * time.Minute, MaxMessageSize: 1024, Backends: []string{"grpc", "web"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := LoadRemoteConfig(providerFrom(t, "fsc:\n"+tc.yaml))
			if len(tc.wantErr) > 0 {
				for _, want := range tc.wantErr {
					require.ErrorContains(t, err, want)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// The backends can be set entirely from the environment.
func TestLoadRemoteConfigFromEnv(t *testing.T) { //nolint:paralleltest // t.Setenv
	t.Setenv("CORE_FSC_VIEW_REMOTE_ENABLED", "true")
	t.Setenv("CORE_FSC_VIEW_REMOTE_BACKENDS", "grpc,web")
	got, err := LoadRemoteConfig(providerFrom(t, "fsc:\n  grpc:\n    enabled: true\n  web:\n    enabled: true\n"))
	require.NoError(t, err)
	require.Equal(t, []string{"grpc", "web"}, got.Backends)
}
