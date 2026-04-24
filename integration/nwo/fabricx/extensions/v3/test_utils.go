/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package v3

import (
	"path"
)

const (
	CommitterVersion        = "v3"
	ScalableCommitterImage  = "hyperledger/fabric-x-committer-test-node:0.1.9"
	SidecarDefaultPort      = "4001/tcp"
	QueryServiceDefaultPort = "7001/tcp"
)

func ContainerCmd(tlsEnabled bool) []string {
	if tlsEnabled {
		return []string{"run", "db", "orderer", "committer"}
	}
	return []string{"run", "db", "orderer", "committer", "--insecure"}
}

func ContainerEnvVars(scMSPDir, scTLSDir, scMSPID, channelName, ordererEndpoint string, tlsEnabled bool, ordererTLSCACert string) []string {
	env := []string{
		"SC_SIDECAR_LOGGING_LOGSPEC=debug",
		"SC_SIDECAR_ORDERER_CHANNEL_ID=" + channelName,
		"SC_SIDECAR_ORDERER_SIGNED_ENVELOPES=true",
		"SC_SIDECAR_ORDERER_IDENTITY_MSP_ID=" + scMSPID,
		"SC_SIDECAR_ORDERER_IDENTITY_MSP_DIR=" + scMSPDir,
		"SC_QUERY_SERVICE_SERVER_ENDPOINT=:7001",
		"SC_QUERY_SERVICE_LOGGING_LOGSPEC=DEBUG",
		"SC_COORDINATOR_LOGGING_LOGSPEC=DEBUG",
		"SC_ORDERER_LOGGING_LOGSPEC=debug",
		"SC_ORDERER_BLOCK_SIZE=1",
		"SC_VC_LOGGING_LOGSPEC=DEBUG",
		"SC_VERIFIER_LOGGING_LOGSPEC=INFO",
		"SC_SIDECAR_SERVER_ENDPOINT=:4001",
		"SC_SIDECAR_SERVER_MAX_CONCURRENT_STREAMS=0",
	}
	if tlsEnabled {
		env = append(env,
			// Orderer TLS (Sidecar as client to Orderer)
			"SC_SIDECAR_ORDERER_TLS_MODE=mtls",
			"SC_ORDERER_TLS_MODE=mtls",
			"SC_SIDECAR_ORDERER_TLS_CERT_FILE="+path.Join(scTLSDir, "server.crt"),
			"SC_SIDECAR_ORDERER_TLS_KEY_FILE="+path.Join(scTLSDir, "server.key"),
			"SC_SIDECAR_ORDERER_TLS_ROOT_CERT_FILE="+ordererTLSCACert,

			// Orderer Server (Internal Orderer)
			"SC_SIDECAR_ORDERER_GENERAL_TLS_ENABLED=true",
			"SC_ORDERER_GENERAL_TLS_ENABLED=true",
			"SC_SIDECAR_ORDERER_GENERAL_TLS_CERTIFICATE="+path.Join(scTLSDir, "server.crt"),
			"SC_ORDERER_GENERAL_TLS_CERTIFICATE="+path.Join(scTLSDir, "server.crt"),
			"SC_SIDECAR_ORDERER_GENERAL_TLS_PRIVATE_KEY="+path.Join(scTLSDir, "server.key"),
			"SC_ORDERER_GENERAL_TLS_PRIVATE_KEY="+path.Join(scTLSDir, "server.key"),
			"SC_SIDECAR_ORDERER_GENERAL_TLS_ROOTCAS="+path.Join(scTLSDir, "ca.crt"),
			"SC_ORDERER_GENERAL_TLS_ROOTCAS="+path.Join(scTLSDir, "ca.crt"),

			// Query Service TLS
			"SC_SIDECAR_QUERY_SERVICE_SERVER_TLS_MODE=mtls",
			"SC_QUERY_SERVICE_SERVER_TLS_MODE=mtls",
			"SC_SIDECAR_QUERY_SERVICE_SERVER_TLS_CERT_FILE="+path.Join(scTLSDir, "server.crt"),
			"SC_QUERY_SERVICE_SERVER_TLS_CERT_FILE="+path.Join(scTLSDir, "server.crt"),
			"SC_SIDECAR_QUERY_SERVICE_SERVER_TLS_KEY_FILE="+path.Join(scTLSDir, "server.key"),
			"SC_QUERY_SERVICE_SERVER_TLS_KEY_FILE="+path.Join(scTLSDir, "server.key"),
			"SC_SIDECAR_QUERY_SERVICE_SERVER_TLS_CLIENT_CA_FILES="+path.Join(scTLSDir, "ca.crt"),
			"SC_QUERY_SERVICE_SERVER_TLS_CLIENT_CA_FILES="+path.Join(scTLSDir, "ca.crt"),

			// Sidecar Server TLS
			"SC_SIDECAR_SERVER_TLS_MODE=mtls",
			"SC_SIDECAR_SERVER_TLS_CERT_FILE="+path.Join(scTLSDir, "server.crt"),
			"SC_SIDECAR_SERVER_TLS_KEY_FILE="+path.Join(scTLSDir, "server.key"),
		)
	} else {
		env = append(env, "SC_SIDECAR_ORDERER_TLS_MODE=none")
	}
	return env
}
