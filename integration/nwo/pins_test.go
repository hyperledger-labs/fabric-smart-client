/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package nwo_test

import (
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/extensions/scv2"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/monitoring/hle"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/monitoring/monitoring"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/monitoring/otlp"
)

// TestImagePins checks that every image nwo runs is one the Makefile pulls.
func TestImagePins(t *testing.T) {
	t.Parallel()

	makefile, err := os.ReadFile("../../Makefile")
	require.NoError(t, err)
	pins := map[string]string{}
	for _, m := range regexp.MustCompile(`(?m)^([A-Z0-9_]+_VERSION) \?= (\S+)$`).FindAllStringSubmatch(string(makefile), -1) {
		pins[m[1]] = m[2]
	}
	require.NotEmpty(t, pins, "no NAME_VERSION ?= value lines in the Makefile")
	var pulled []string
	for _, m := range regexp.MustCompile(`(?m)^\tdocker pull (\S+)$`).FindAllStringSubmatch(string(makefile), -1) {
		pulled = append(pulled, regexp.MustCompile(`\$\(([A-Z0-9_]+)\)`).ReplaceAllStringFunc(m[1], func(v string) string {
			return pins[v[2:len(v)-1]]
		}))
	}

	images := map[string]string{
		"scv2.DefaultCommitterImage": scv2.DefaultCommitterImage,
		"otlp.JaegerTracing":         otlp.JaegerTracing,
		"monitoring.PrometheusImg":   monitoring.PrometheusImg,
		"monitoring.GrafanaImg":      monitoring.GrafanaImg,
		"hle.Explorer":               hle.Explorer,
		"hle.ExplorerDB":             hle.ExplorerDB,
	}
	for name, ref := range images {
		require.Contains(t, pulled, ref, "%s is not an image the Makefile pulls", name)
	}

	workflow, err := os.ReadFile("../../.github/workflows/golangci-lint.yml")
	require.NoError(t, err)
	versions := regexp.MustCompile(`(?m)^\s+version:\s+(\S+)$`).FindAllStringSubmatch(string(workflow), -1)
	require.Len(t, versions, 1, "golangci-lint.yml should set version: once")
	require.Equal(t, pins["GOLANGCI_LINT_VERSION"], versions[0][1], "GOLANGCI_LINT_VERSION does not match version: in golangci-lint.yml")
}
