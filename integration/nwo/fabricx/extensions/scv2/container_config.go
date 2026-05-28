/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scv2

import (
	"path"
)

const (
	scalableCommitterImage = "hyperledger/fabric-x-committer-test-node:1.0.0"
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
}

func containerCmd(cfg containerConfig) []string {
	cmd := []string{"run", "db", "orderer", "committer"}

	if !cfg.TLSEnabled {
		cmd = append(cmd, "--insecure")
	}

	return cmd
}

func containerEnvVars(cfg containerConfig) []string {
	env := []string{
		"SC_SIDECAR_LOGGING_LOGSPEC=info:grpc=error",
		"SC_SIDECAR_ORDERER_CHANNEL_ID=" + cfg.ChannelName,
		"SC_SIDECAR_ORDERER_SIGNED_ENVELOPES=true",
		"SC_SIDECAR_ORDERER_IDENTITY_MSP_ID=" + cfg.SidecarMSPID,
		"SC_SIDECAR_ORDERER_IDENTITY_MSP_DIR=" + cfg.SidecarMSPDir,
		"SC_SIDECAR_SERVER_ENDPOINT=" + cfg.SidecarServerEndpoint,
		"SC_SIDECAR_SERVER_MAX_CONCURRENT_STREAMS=0",
		"SC_QUERY_SERVER_ENDPOINT=" + cfg.QueryServerEndpoint,
		"SC_QUERY_LOGGING_LOGSPEC=info:grpc=error",
		"SC_COORDINATOR_LOGGING_LOGSPEC=info:grpc=error",
		"SC_ORDERER_LOGGING_LOGSPEC=info:grpc=error",
		"SC_ORDERER_BLOCK_SIZE=1",
		"SC_ORDERER_SERVER_ENDPOINT=" + cfg.OrdererServerEndpoint,
		"SC_VC_LOGGING_LOGSPEC=info:grpc=error",
		"SC_VERIFIER_LOGGING_LOGSPEC=info:grpc=error",
	}
	if cfg.TLSEnabled {
		env = append(env, tlsEnvGroup("SC_SIDECAR_SERVER", cfg.SidecarTLSDir, cfg.CertsBundle)...)
		env = append(env, tlsEnvGroup("SC_SIDECAR_ORDERER", cfg.SidecarTLSDir, cfg.CertsBundle)...)
		env = append(env, tlsEnvGroup("SC_QUERY_SERVER", cfg.SidecarTLSDir, cfg.CertsBundle)...)
		env = append(env, tlsEnvGroup("SC_ORDERER_SERVER", cfg.OrdererTLSDir, cfg.CertsBundle)...)
	}
	return env
}

func tlsEnvGroup(prefix, certDir, caBundle string) []string {
	return []string{
		prefix + "_TLS_MODE=mtls",
		prefix + "_TLS_CERT_PATH=" + path.Join(certDir, "server.crt"),
		prefix + "_TLS_KEY_PATH=" + path.Join(certDir, "server.key"),
		prefix + "_TLS_CA_CERT_PATHS=" + caBundle,
	}
}
