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

	ScrapeConfig struct {
		JobName       string         `yaml:"job_name"`
		StaticConfigs []StaticConfig `yaml:"static_configs"`
	}

	Prometheus struct {
		Global        Global         `yaml:"global"`
		ScrapeConfigs []ScrapeConfig `yaml:"scrape_configs"`
	}
)
