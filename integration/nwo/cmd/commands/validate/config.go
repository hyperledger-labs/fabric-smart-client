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

	// One call resolves and validates every fsc.* TLS surface, requires fsc.web.address and
	// rejects removed keys, through exactly the code the node runs at startup, so validate
	// cannot accept a configuration the node rejects.
	if err := sdk.CheckTLSConfig(configService); err != nil {
		return report, errors.Wrap(err, "invalid fsc listener configuration")
	}
	// Reported per surface, not as one line: a disabled listener was not validated, and a
	// report that claims otherwise is worse than no report.
	for _, surface := range []string{"fsc.grpc", "fsc.web"} {
		if configService.GetBool(surface + ".enabled") {
			report.Checks = append(report.Checks, fmt.Sprintf("validated %s server configuration", surface))
		}
	}
	warning, err := sdk.OperationsClientAuthWarning(configService)
	if err != nil {
		return report, errors.Wrap(err, "invalid fsc.metrics configuration")
	}
	if warning != "" {
		report.Checks = append(report.Checks, "warning: "+warning)
	}

	if configService.IsSet("fsc.tracing") {
		var tracingConfig tracing.Config
		if err := configService.UnmarshalKey("fsc.tracing", &tracingConfig); err != nil {
			return report, errors.Wrap(err, "invalid fsc.tracing configuration")
		}
		if err := tracingConfig.Validate(); err != nil {
			return report, errors.Wrap(err, "invalid fsc.tracing configuration")
		}
		report.Checks = append(report.Checks, "validated fsc.tracing configuration")
	}

	return report, nil
}
