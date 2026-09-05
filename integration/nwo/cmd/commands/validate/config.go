/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package validate

import (
	"fmt"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	fabriccore "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	sdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
)

// Report summarizes the checks performed during configuration validation.
type Report struct {
	Checks []string
}

// String returns a human-readable representation of the validation report.
func (r Report) String() string {
	var builder strings.Builder
	builder.WriteString("configuration is valid")
	for _, check := range r.Checks {
		builder.WriteString("\n- ")
		builder.WriteString(check)
	}
	return builder.String()
}

// ValidateConfig validates the FSC configuration rooted at the given path.
func ValidateConfig(confPath string) (Report, error) {
	report := Report{}

	n, err := node.NewFromConfPathE(confPath)
	if err != nil {
		return report, errors.Wrap(err, "invalid node configuration")
	}
	report.Checks = append(report.Checks, fmt.Sprintf("loaded node configuration for [%s]", n.ID()))

	configService, err := config.NewProvider(confPath)
	if err != nil {
		return report, errors.Wrap(err, "invalid configuration path")
	}

	if configService.IsSet("fabric") {
		fabricConfig, err := fabriccore.NewConfig(configService)
		if err != nil {
			return report, errors.Wrap(err, "invalid fabric configuration")
		}
		report.Checks = append(report.Checks, fmt.Sprintf("validated fabric networks [%s]", strings.Join(fabricConfig.Names(), ", ")))
	}

	if configService.GetBool("fsc.grpc.enabled") && configService.GetString("fsc.grpc.address") == "" {
		return report, errors.New("invalid fsc.grpc configuration: missing address")
	}
	if configService.GetBool("fsc.web.enabled") && configService.GetString("fsc.web.address") == "" {
		return report, errors.New("invalid fsc.web configuration: missing address")
	}

	// One call resolves and validates every fsc.* TLS surface and rejects removed keys,
	// through exactly the code the node runs at startup. This used to hand-roll the web
	// half, which is how the two drifted apart.
	if err := sdk.CheckTLSConfig(configService); err != nil {
		return report, errors.Wrap(err, "invalid TLS configuration")
	}
	// Reported per surface, not as one line: a disabled listener was not validated, and a
	// report that claims otherwise is worse than no report.
	for _, surface := range []string{"fsc.grpc", "fsc.web"} {
		if configService.GetBool(surface + ".enabled") {
			report.Checks = append(report.Checks, fmt.Sprintf("validated %s server configuration", surface))
		}
	}

	if configService.IsSet("fsc.tracing") {
		var tracingConfig tracing.Config
		if err := configService.UnmarshalKey("fsc.tracing", &tracingConfig); err != nil {
			return report, errors.Wrap(err, "invalid fsc.tracing configuration")
		}
		if err := validateTracingConfig(tracingConfig); err != nil {
			return report, errors.Wrap(err, "invalid fsc.tracing configuration")
		}
		report.Checks = append(report.Checks, "validated fsc.tracing configuration")
	}

	return report, nil
}

func validateTracingConfig(c tracing.Config) error {
	switch c.Provider {
	case "", tracing.None, tracing.Console:
		return nil
	case tracing.File:
		if c.File.Path == "" {
			return errors.New("file provider requires fsc.tracing.file.path")
		}
		return nil
	case tracing.Otlp:
		if c.Otlp.Address == "" {
			return errors.New("otlp provider requires fsc.tracing.otlp.address")
		}
		return nil
	default:
		return errors.Errorf("unsupported provider [%s]", c.Provider)
	}
}
