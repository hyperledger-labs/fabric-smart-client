/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package monitoring

type (
	Global struct {
		ScrapeInterval     string `yaml:"scrape_interval"`
		EvaluationInterval string `yaml:"evaluation_interval"`
	}

	StaticConfig struct {
		Targets []string `yaml:"targets"`
	}

	TLSConfig struct {
		CAFile             string `yaml:"ca_file"`
		CertFile           string `yaml:"cert_file"`
		KeyFile            string `yaml:"key_file"`
		ServerName         string `yaml:"server_name"`
		InsecureSkipVerify bool   `yaml:"insecure_skip_verify"`
	}

	ScrapeConfig struct {
		JobName       string         `yaml:"job_name"`
		Scheme        string         `yaml:"scheme"`
		StaticConfigs []StaticConfig `yaml:"static_configs"`
		TLSConfig     *TLSConfig     `yaml:"tls_config,omitempty"`
	}

	Prometheus struct {
		Global        Global         `yaml:"global"`
		ScrapeConfigs []ScrapeConfig `yaml:"scrape_configs"`
	}
)
