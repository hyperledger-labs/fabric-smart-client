/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package validate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateConfig_Success(t *testing.T) {
	t.Parallel()

	confPath := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(confPath, "core.yaml"), []byte(validConfigYAML(t)), 0o600))

	report, err := ValidateConfig(confPath)
	require.NoError(t, err)
	require.Contains(t, report.String(), "configuration is valid")
	require.Contains(t, report.String(), "validated fsc.grpc server configuration")
	require.Contains(t, report.String(), "validated fsc.web server configuration")
	require.Contains(t, report.String(), "validated fabric networks [default]")
}

func TestValidateConfig_MissingGRPCAddress(t *testing.T) {
	t.Parallel()

	confPath := t.TempDir()
	raw := strings.Replace(validConfigYAML(t), "    address: 127.0.0.1:7051\n", "", 1)
	require.NoError(t, os.WriteFile(filepath.Join(confPath, "core.yaml"), []byte(raw), 0o600))

	_, err := ValidateConfig(confPath)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing address")
}

func TestValidateConfig_InvalidTracingConfig(t *testing.T) {
	t.Parallel()

	confPath := t.TempDir()
	raw := strings.Replace(validConfigYAML(t), "fabric:\n", "  tracing:\n    provider: otlp\nfabric:\n", 1)
	require.NoError(t, os.WriteFile(filepath.Join(confPath, "core.yaml"), []byte(raw), 0o600))

	_, err := ValidateConfig(confPath)
	require.Error(t, err)
	require.Contains(t, err.Error(), "fsc.tracing")
	require.Contains(t, err.Error(), "requires fsc.tracing.otlp.address")
}

func validConfigYAML(t *testing.T) string {
	t.Helper()

	abs := func(path string) string {
		p, err := filepath.Abs(path)
		require.NoError(t, err)
		return p
	}
	cert := abs(filepath.Join("testdata", "tls", "server.crt"))
	key := abs(filepath.Join("testdata", "tls", "server.key"))
	ca := abs(filepath.Join("testdata", "tls", "ca.crt"))

	return "" +
		"fsc:\n" +
		"  id: node1\n" +
		"  grpc:\n" +
		"    enabled: true\n" +
		"    address: 127.0.0.1:7051\n" +
		"    tls:\n" +
		"      enabled: true\n" +
		"      cert:\n" +
		"        file: " + cert + "\n" +
		"      key:\n" +
		"        file: " + key + "\n" +
		"  web:\n" +
		"    enabled: true\n" +
		"    address: 127.0.0.1:8443\n" +
		"    tls:\n" +
		"      enabled: true\n" +
		"      cert:\n" +
		"        file: " + cert + "\n" +
		"      key:\n" +
		"        file: " + key + "\n" +
		"      clientRootCAs:\n" +
		"        files:\n" +
		"          - " + ca + "\n" +
		"fabric:\n" +
		"  default:\n" +
		"    default: true\n" +
		"    driver: generic\n"
}
