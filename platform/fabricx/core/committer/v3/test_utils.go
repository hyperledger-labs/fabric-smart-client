/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package v3

const (
	CommitterVersion        = "v3"
	ScalableCommitterImage  = "hyperledger/fabric-x-committer-test-node:0.1.7"
	SidecarDefaultPort      = "4001/tcp"
	QueryServiceDefaultPort = "7001/tcp"
)

var ContainerCmd = []string{"run", "db", "orderer", "committer"}

func ContainerEnvVars(peerMSPDir, scMSPID, channelName, ordererEndpoint string) []string {
	return []string{
		"SC_SIDECAR_ORDERER_IDENTITY_MSP_DIR=" + peerMSPDir,
		"SC_SIDECAR_ORDERER_IDENTITY_MSP_ID=" + scMSPID,
		"SC_SIDECAR_ORDERER_CHANNEL_ID=" + channelName,
		"SC_SIDECAR_ORDERER_SIGNED_ENVELOPES=true",
		"SC_SIDECAR_LOGGING_LEVEL=DEBUG",
		"SC_QUERY_SERVICE_SERVER_ENDPOINT=:7001",
		"SC_QUERY_SERVICE_LOGGING_LEVEL=DEBUG",
		"SC_COORDINATOR_LOGGING_LEVEL=DEBUG",
		"SC_ORDERER_LOGGING_LEVEL=DEBUG",
		"SC_ORDERER_BLOCK_SIZE=1",
		"SC_VC_LOGGING_LEVEL=DEBUG",
		"SC_VERIFIER_LOGGING_LEVEL=INFO",
	}
}
