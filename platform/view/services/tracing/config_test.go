/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracing_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
)

func callGetPackageName() string {
	return tracing.GetPackageName()
}

func TestGetPackageName(t *testing.T) {
	t.Parallel()

	pkg := callGetPackageName()

	require.Equal(t, "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing_test", pkg)
}

type ConfigHandler struct{}

func (h *ConfigHandler) sendResponseLikeMethod() string {
	return callGetPackageName()
}

func TestGetPackageName_FromStructMethod(t *testing.T) {
	t.Parallel()

	handler := &ConfigHandler{}
	pkg := handler.sendResponseLikeMethod()

	require.Equal(t, "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing_test", pkg)
}

func TestWithMetricsOpts_CustomSubsystem(t *testing.T) {
	t.Parallel()

	opts := tracing.MetricsOpts{
		Subsystem:  "custom_subsystem",
		LabelNames: []string{"label1", "label2"},
	}

	tracerOpt := tracing.WithMetricsOpts(opts)

	config := trace.NewTracerConfig(tracerOpt)
	attrs := config.InstrumentationAttributes()

	namespaceVal, ok := attrs.Value("namespace")
	require.True(t, ok)
	require.Equal(t, "fsc_view_services", namespaceVal.AsString())

	subsystemVal, ok := attrs.Value("subsystem")
	require.True(t, ok)
	require.Equal(t, "custom_subsystem", subsystemVal.AsString())

	labelNamesVal, ok := attrs.Value("label_names")
	require.True(t, ok)
	require.Equal(t, []string{"label1", "label2"}, labelNamesVal.AsStringSlice())
}

func TestWithMetricsOpts_EmptyOptions(t *testing.T) {
	t.Parallel()

	opts := tracing.MetricsOpts{}

	tracerOpt := tracing.WithMetricsOpts(opts)

	config := trace.NewTracerConfig(tracerOpt)
	attrs := config.InstrumentationAttributes()

	namespaceVal, ok := attrs.Value("namespace")
	require.True(t, ok)
	require.Equal(t, "fsc_view_services", namespaceVal.AsString())

	subsystemVal, ok := attrs.Value("subsystem")
	require.True(t, ok)
	require.Equal(t, "tracing_test", subsystemVal.AsString())

	labelNamesVal, ok := attrs.Value("label_names")
	require.True(t, ok)
	require.Empty(t, labelNamesVal.AsStringSlice())
}

func TestRegisterReplacer_PanicsOnDuplicate(t *testing.T) { //nolint:paralleltest // modifies package-global replacers state; must not run in parallel with other tests
	require.Panics(t, func() {
		tracing.RegisterReplacer("github.com_hyperledger-labs_fabric-smart-client_platform", "fsc")
	})
}

func TestReplacers_ContainsDefault(t *testing.T) {
	t.Parallel()

	r := tracing.Replacers()
	require.Contains(t, r, "github.com_hyperledger-labs_fabric-smart-client_platform")
}

func TestReplacers_ReturnsCopy_ConsumerCannotMutateInternalState(t *testing.T) {
	t.Parallel()

	// Grab a snapshot, mutate it aggressively, then grab a second snapshot and
	// verify the mutations did not leak into the package-level replacers map.
	snapshot := tracing.Replacers()
	snapshot["injected-key"] = "injected-value"
	delete(snapshot, "github.com_hyperledger-labs_fabric-smart-client_platform")

	secondSnapshot := tracing.Replacers()
	require.NotContains(t, secondSnapshot, "injected-key",
		"Replacers() must return a copy: mutations by callers must not leak")
	require.Contains(t, secondSnapshot, "github.com_hyperledger-labs_fabric-smart-client_platform",
		"Replacers() must return a copy: deletions by callers must not leak")
}
