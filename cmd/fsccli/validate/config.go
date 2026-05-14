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
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	webserver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server"
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

	if configService.GetBool("fsc.grpc.enabled") {
		if configService.GetString("fsc.grpc.address") == "" {
			return report, errors.New("invalid fsc.grpc configuration: missing address")
		}

		if _, err := sdk.NewServerConfig(configService); err != nil {
			return report, errors.Wrap(err, "invalid fsc.grpc configuration")
		}
		report.Checks = append(report.Checks, "validated fsc.grpc server configuration")
	}

	if configService.GetBool("fsc.web.enabled") {
		if configService.GetString("fsc.web.address") == "" {
			return report, errors.New("invalid fsc.web configuration: missing address")
		}

		tlsConfig := webserver.TLS{
			Enabled:           configService.GetBool("fsc.web.tls.enabled"),
			CertFile:          configService.GetPath("fsc.web.tls.cert.file"),
			KeyFile:           configService.GetPath("fsc.web.tls.key.file"),
			ClientAuth:        configService.GetBool("fsc.web.tls.clientAuthRequired"),
			ClientCACertFiles: translatePaths(configService, configService.GetStringSlice("fsc.web.tls.clientRootCAs.files")),
		}
		if _, err := tlsConfig.Config(); err != nil {
			return report, errors.Wrap(err, "invalid fsc.web TLS configuration")
		}
		report.Checks = append(report.Checks, "validated fsc.web server configuration")
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

func translatePaths(configService *config.Provider, paths []string) []string {
	translated := make([]string, 0, len(paths))
	for _, path := range paths {
		translated = append(translated, configService.TranslatePath(path))
	}
	return translated
}
