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

func TestGetPackageName(t *testing.T) { //nolint:paralleltest
	t.Parallel()

	pkg := callGetPackageName()

	require.Equal(t, "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing_test", pkg)
}

type ConfigHandler struct{}

func (h *ConfigHandler) sendResponseLikeMethod() string {
	return callGetPackageName()
}

func TestGetPackageName_FromStructMethod(t *testing.T) { //nolint:paralleltest
	t.Parallel()

	handler := &ConfigHandler{}
	pkg := handler.sendResponseLikeMethod()

	require.Equal(t, "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing_test", pkg)
}

func TestWithMetricsOpts_CustomSubsystem(t *testing.T) { //nolint:paralleltest
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

func TestWithMetricsOpts_EmptyOptions(t *testing.T) { //nolint:paralleltest
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

func TestParseFullPkgName(t *testing.T) {
	t.Parallel()

	opts := tracing.WithMetricsOpts(tracing.MetricsOpts{
		LabelNames: []string{"op", "status"},
	})

	config := trace.NewTracerConfig(opts)
	attrs := config.InstrumentationAttributes()

	namespaceVal, ok := attrs.Value("namespace")
	require.True(t, ok)
	require.NotEmpty(t, namespaceVal.AsString())

	subsystemVal, ok := attrs.Value("subsystem")
	require.True(t, ok)
	require.NotEmpty(t, subsystemVal.AsString())

	labelNamesVal, ok := attrs.Value("label_names")
	require.True(t, ok)
	require.Equal(t, []string{"op", "status"}, labelNamesVal.AsStringSlice())
}

func TestWithMetricsOpts_CustomNamespace(t *testing.T) {
	t.Parallel()

	customNamespace := "custom_namespace"
	opts := tracing.WithMetricsOpts(tracing.MetricsOpts{
		Namespace: customNamespace,
	})

	config := trace.NewTracerConfig(opts)
	attrs := config.InstrumentationAttributes()

	namespaceVal, ok := attrs.Value("namespace")
	require.True(t, ok)
	require.Equal(t, customNamespace, namespaceVal.AsString())
}

func TestWithMetricsOpts_EmptyLabelNames(t *testing.T) {
	t.Parallel()

	opts := tracing.WithMetricsOpts(tracing.MetricsOpts{
		LabelNames: []string{},
	})

	config := trace.NewTracerConfig(opts)
	attrs := config.InstrumentationAttributes()

	labelNamesVal, ok := attrs.Value("label_names")
	require.True(t, ok)
	require.Empty(t, labelNamesVal.AsStringSlice())
}

func TestReplacers_InitialState(t *testing.T) {
	t.Parallel()

	replacers := tracing.Replacers()
	require.NotNil(t, replacers)
	require.Contains(t, replacers, "github.com_hyperledger-labs_fabric-smart-client_platform")
}

func TestReplacers_ContainsExpectedMapping(t *testing.T) {
	t.Parallel()

	replacers := tracing.Replacers()
	value, exists := replacers["github.com_hyperledger-labs_fabric-smart-client_platform"]
	require.True(t, exists)
	require.Equal(t, "fsc", value)
}

func TestGetPackageName_Caller(t *testing.T) {
	t.Parallel()

	pkg := getTestPackageName()
	require.Contains(t, pkg, "tracing_test")
}

func getTestPackageName() string {
	return tracing.GetPackageName()
}
