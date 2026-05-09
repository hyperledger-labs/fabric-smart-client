/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package v3

import (
	"path"
	"strings"
)

const (
	CommitterVersion        = "v3"
	ScalableCommitterImage  = "hyperledger/fabric-x-committer-test-node:1.0.0"
	SidecarDefaultPort      = "4001/tcp"
	QueryServiceDefaultPort = "7001/tcp"
)

func ContainerCmd(tlsEnabled bool) []string {
	if tlsEnabled {
		return []string{"run", "db", "orderer", "committer"}
	}
	return []string{"run", "db", "orderer", "committer", "--insecure"}
}

func ContainerEnvVars(scMSPDir, scTLSDir, scMSPID, channelName, ordererEndpoint string, tlsEnabled bool, ordererTLSCACert, clientCACertsBundle string) []string {
	parts := strings.Split(ordererEndpoint, ":")
	host := parts[0]
	port := parts[1]

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
			"SC_SIDECAR_ORDERER_TLS_ROOT_CERT_FILE="+clientCACertsBundle,
			"SC_SIDECAR_ORDERER_SERVER_ENDPOINT="+ordererEndpoint,
			"SC_ORDERER_ORGANIZATIONS_org0_ENDPOINTS_0_HOST="+host,
			"SC_ORDERER_ORGANIZATIONS_org0_ENDPOINTS_0_PORT="+port,
			"SC_ORDERER_ORGANIZATIONS_ordererorg_ENDPOINTS_0_HOST="+host,
			"SC_ORDERER_ORGANIZATIONS_ordererorg_ENDPOINTS_0_PORT="+port,
			"SC_SIDECAR_ORDERER_ORGANIZATIONS_org0_ENDPOINTS_0_HOST="+host,
			"SC_SIDECAR_ORDERER_ORGANIZATIONS_org0_ENDPOINTS_0_PORT="+port,
			"SC_SIDECAR_ORDERER_ORGANIZATIONS_ordererorg_ENDPOINTS_0_HOST="+host,
			"SC_SIDECAR_ORDERER_ORGANIZATIONS_ordererorg_ENDPOINTS_0_PORT="+port,

			// Orderer Server (Internal Orderer)
			"SC_SIDECAR_ORDERER_GENERAL_TLS_ENABLED=true",
			"SC_ORDERER_GENERAL_TLS_ENABLED=true",
			"SC_SIDECAR_ORDERER_GENERAL_TLS_CERTIFICATE="+path.Join(scTLSDir, "server.crt"),
			"SC_ORDERER_GENERAL_TLS_CERTIFICATE="+path.Join(scTLSDir, "server.crt"),
			"SC_SIDECAR_ORDERER_GENERAL_TLS_PRIVATE_KEY="+path.Join(scTLSDir, "server.key"),
			"SC_ORDERER_GENERAL_TLS_PRIVATE_KEY="+path.Join(scTLSDir, "server.key"),
			"SC_SIDECAR_ORDERER_GENERAL_TLS_ROOTCAS="+clientCACertsBundle,
			"SC_ORDERER_GENERAL_TLS_ROOTCAS="+clientCACertsBundle,

			// Query Service TLS
			"SC_SIDECAR_QUERY_SERVICE_SERVER_TLS_MODE=mtls",
			"SC_QUERY_SERVICE_SERVER_TLS_MODE=mtls",
			"SC_SIDECAR_QUERY_SERVICE_SERVER_TLS_CERT_FILE="+path.Join(scTLSDir, "server.crt"),
			"SC_QUERY_SERVICE_SERVER_TLS_CERT_FILE="+path.Join(scTLSDir, "server.crt"),
			"SC_SIDECAR_QUERY_SERVICE_SERVER_TLS_KEY_FILE="+path.Join(scTLSDir, "server.key"),
			"SC_QUERY_SERVICE_SERVER_TLS_KEY_FILE="+path.Join(scTLSDir, "server.key"),
			"SC_SIDECAR_QUERY_SERVICE_SERVER_TLS_CLIENT_CA_FILES="+clientCACertsBundle,
			"SC_QUERY_SERVICE_SERVER_TLS_CLIENT_CA_FILES="+clientCACertsBundle,
			"SC_SIDECAR_QUERY_SERVICE_TLS_CLIENT_CA_FILES="+clientCACertsBundle,
			"SC_QUERY_SERVICE_TLS_CLIENT_CA_FILES="+clientCACertsBundle,
			"SC_SIDECAR_QUERY_SERVICE_SERVER_TLS_CACERTPATHS_0="+clientCACertsBundle,
			"SC_QUERY_SERVICE_SERVER_TLS_CACERTPATHS_0="+clientCACertsBundle,

			// Sidecar Server TLS
			"SC_SIDECAR_SERVER_TLS_MODE=mtls",
			"SC_SIDECAR_SERVER_TLS_CERT_FILE=/server-certs/public-key.pem",
			"SC_SIDECAR_SERVER_TLS_KEY_FILE=/server-certs/private-key.pem",
			"SC_SIDECAR_SERVER_TLS_CLIENT_CA_FILES=/server-certs/ca-certificate.pem",
			"SC_SIDECAR_SERVER_TLS_CACERTPATHS_0=/server-certs/ca-certificate.pem",

			"SC_SERVER_TLS_MODE=mtls",
			"SC_SERVER_TLS_CERT_FILE=/server-certs/public-key.pem",
			"SC_SERVER_TLS_KEY_FILE=/server-certs/private-key.pem",
			"SC_SERVER_TLS_CLIENT_CA_FILES=/server-certs/ca-certificate.pem",
			"SC_SERVER_TLS_CACERTPATHS_0=/server-certs/ca-certificate.pem",

			"SC_SIDECAR_QUERY_SERVICE_SERVER_TLS_MODE=mtls",
			"SC_SIDECAR_QUERY_SERVICE_SERVER_TLS_CERT_FILE=/server-certs/public-key.pem",
			"SC_SIDECAR_QUERY_SERVICE_SERVER_TLS_KEY_FILE=/server-certs/private-key.pem",
			"SC_SIDECAR_QUERY_SERVICE_SERVER_TLS_CLIENT_CA_FILES=/server-certs/ca-certificate.pem",
			"SC_SIDECAR_QUERY_SERVICE_SERVER_TLS_CACERTPATHS_0=/server-certs/ca-certificate.pem",

			"SC_QUERY_SERVICE_SERVER_TLS_MODE=mtls",
			"SC_QUERY_SERVICE_SERVER_TLS_CERT_FILE=/server-certs/public-key.pem",
			"SC_QUERY_SERVICE_SERVER_TLS_KEY_FILE=/server-certs/private-key.pem",
			"SC_QUERY_SERVICE_SERVER_TLS_CLIENT_CA_FILES=/server-certs/ca-certificate.pem",
			"SC_QUERY_SERVICE_SERVER_TLS_CACERTPATHS_0=/server-certs/ca-certificate.pem",

			"SC_SIDECAR_ORDERER_TLS_MODE=mtls",
			"SC_SIDECAR_ORDERER_TLS_CERT_FILE=/client-certs/public-key.pem",
			"SC_SIDECAR_ORDERER_TLS_KEY_FILE=/client-certs/private-key.pem",
			"SC_SIDECAR_ORDERER_TLS_ROOT_CERT_FILE=/client-certs/ca-certificate.pem",

			"SC_COMMITTER_TLS_MODE=mtls",
			"SC_COMMITTER_TLS_CACERTPATHS_0=/server-certs/ca-certificate.pem",
			"SC_VERIFIER_TLS_MODE=mtls",
			"SC_VERIFIER_TLS_CACERTPATHS_0=/server-certs/ca-certificate.pem",
			"SC_VALIDATORCOMMITTER_TLS_MODE=mtls",
			"SC_VALIDATORCOMMITTER_TLS_CACERTPATHS_0=/server-certs/ca-certificate.pem",
		)
	} else {
		env = append(env,
			"SC_SIDECAR_ORDERER_TLS_MODE=none",
			"SC_ORDERER_TLS_MODE=none",
			"SC_SIDECAR_SERVER_TLS_MODE=none",
			"SC_SERVER_TLS_MODE=none",
			"SC_COMMITTER_TLS_MODE=none",
			"SC_VERIFIER_TLS_MODE=none",
			"SC_VALIDATORCOMMITTER_TLS_MODE=none",
			"SC_QUERY_SERVICE_SERVER_TLS_MODE=none",
		)
	}
	return env
}
