/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracing_test

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	. "github.com/onsi/gomega"
	"go.opentelemetry.io/otel/trace"
)

func callGetPackageName() string {
	return tracing.GetPackageName()
}

func TestGetPackageName(t *testing.T) {
	RegisterTestingT(t)

	pkg := callGetPackageName()

	Expect(pkg).To(Equal("github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing_test"))
}

type ConfigHandler struct{}

func (h *ConfigHandler) sendResponseLikeMethod() string {
	return callGetPackageName()
}

func TestGetPackageName_FromStructMethod(t *testing.T) {
	RegisterTestingT(t)

	handler := &ConfigHandler{}
	pkg := handler.sendResponseLikeMethod()

	Expect(pkg).To(Equal("github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing_test"))
}

func TestWithMetricsOpts_CustomSubsystem(t *testing.T) {
	RegisterTestingT(t)

	opts := tracing.MetricsOpts{
		Subsystem:  "custom_subsystem",
		LabelNames: []string{"label1", "label2"},
	}

	tracerOpt := tracing.WithMetricsOpts(opts)

	config := trace.NewTracerConfig(tracerOpt)
	attrs := config.InstrumentationAttributes()

	namespaceVal, ok := attrs.Value("namespace")
	Expect(ok).To(BeTrue())
	Expect(namespaceVal.AsString()).To(Equal("fsc_view_services"))

	subsystemVal, ok := attrs.Value("subsystem")
	Expect(ok).To(BeTrue())
	Expect(subsystemVal.AsString()).To(Equal("custom_subsystem"))

	labelNamesVal, ok := attrs.Value("label_names")
	Expect(ok).To(BeTrue())
	Expect(labelNamesVal.AsStringSlice()).To(Equal([]string{"label1", "label2"}))
}

func TestWithMetricsOpts_EmptyOptions(t *testing.T) {
	RegisterTestingT(t)

	opts := tracing.MetricsOpts{}

	tracerOpt := tracing.WithMetricsOpts(opts)

	config := trace.NewTracerConfig(tracerOpt)
	attrs := config.InstrumentationAttributes()

	namespaceVal, ok := attrs.Value("namespace")
	Expect(ok).To(BeTrue())
	Expect(namespaceVal.AsString()).To(Equal("fsc_view_services"))

	subsystemVal, ok := attrs.Value("subsystem")
	Expect(ok).To(BeTrue())
	Expect(subsystemVal.AsString()).To(Equal("tracing_test"))

	labelNamesVal, ok := attrs.Value("label_names")
	Expect(ok).To(BeTrue())
	Expect(labelNamesVal.AsStringSlice()).To(BeEmpty())

}
