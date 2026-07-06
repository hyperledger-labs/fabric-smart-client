/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scv2

import (
	"cmp"
	"path"
	"sort"
)

const (
	committerImageName = "hyperledger/fabric-x-committer-test-node:1.0.3"
)

type containerConfig struct {
	ChannelName           string
	SidecarMSPDir         string
	SidecarMSPID          string
	SidecarServerEndpoint string
	QueryServerEndpoint   string
	OrdererServerEndpoint string
	TLSEnabled            bool
	CertsBundle           string
	SidecarTLSDir         string
	OrdererTLSDir         string
	Image                 string
	EnvVarOverrides       map[string]string
}

func containerImage(cfg containerConfig) string {
	return cmp.Or(cfg.Image, committerImageName)
}

func containerCmd(cfg containerConfig) []string {
	cmd := []string{"run", "db", "orderer", "committer"}

	if !cfg.TLSEnabled {
		cmd = append(cmd, "--insecure")
	}

	return cmd
}

func containerEnvVars(cfg containerConfig) []string {
	env := map[string]string{
		"SC_SIDECAR_LOGGING_LOGSPEC":               "info:grpc=error",
		"SC_SIDECAR_ORDERER_CHANNEL_ID":            cfg.ChannelName,
		"SC_SIDECAR_ORDERER_SIGNED_ENVELOPES":      "true",
		"SC_SIDECAR_ORDERER_IDENTITY_MSP_ID":       cfg.SidecarMSPID,
		"SC_SIDECAR_ORDERER_IDENTITY_MSP_DIR":      cfg.SidecarMSPDir,
		"SC_SIDECAR_SERVER_ENDPOINT":               cfg.SidecarServerEndpoint,
		"SC_SIDECAR_SERVER_MAX_CONCURRENT_STREAMS": "0",
		"SC_QUERY_SERVER_ENDPOINT":                 cfg.QueryServerEndpoint,
		"SC_QUERY_LOGGING_LOGSPEC":                 "info:grpc=error",
		"SC_COORDINATOR_LOGGING_LOGSPEC":           "info:grpc=error",
		"SC_ORDERER_LOGGING_LOGSPEC":               "info:grpc=error",
		"SC_ORDERER_BLOCK_SIZE":                    "1",
		"SC_ORDERER_SERVER_ENDPOINT":               cfg.OrdererServerEndpoint,
		"SC_VC_LOGGING_LOGSPEC":                    "info:grpc=error",
		"SC_VERIFIER_LOGGING_LOGSPEC":              "info:grpc=error",
	}
	if cfg.TLSEnabled {
		addTLSEnvGroup(env, "SC_SIDECAR_SERVER", cfg.SidecarTLSDir, cfg.CertsBundle)
		addTLSEnvGroup(env, "SC_SIDECAR_ORDERER", cfg.SidecarTLSDir, cfg.CertsBundle)
		addTLSEnvGroup(env, "SC_QUERY_SERVER", cfg.SidecarTLSDir, cfg.CertsBundle)
		addTLSEnvGroup(env, "SC_ORDERER_SERVER", cfg.OrdererTLSDir, cfg.CertsBundle)
	}

	// Apply overrides: empty string removes the default; non-empty overrides or adds.
	for k, v := range cfg.EnvVarOverrides {
		if v == "" {
			delete(env, k)
		} else {
			env[k] = v
		}
	}

	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	result := make([]string, 0, len(env))
	for _, k := range keys {
		result = append(result, k+"="+env[k])
	}
	return result
}

func addTLSEnvGroup(env map[string]string, prefix, certDir, caBundle string) {
	env[prefix+"_TLS_MODE"] = "mtls"
	env[prefix+"_TLS_CERT_PATH"] = path.Join(certDir, "server.crt")
	env[prefix+"_TLS_KEY_PATH"] = path.Join(certDir, "server.key")
	env[prefix+"_TLS_CA_CERT_PATHS"] = caBundle
}
