/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

import (
	"os"
	"path"
)

const (
	FabricBinsPathEnvKey = "FAB_BINS"
	configtxgenCMD       = "configtxgen"
	configtxlatorCMD     = "configtxlator"
	cryptogenCMD         = "cryptogen"
	discoverCMD          = "discover"
	fabricCaClientCMD    = "fabric-ca-client"
	fabricCaServerCMD    = "fabric-ca-server"
	idemixgenCMD         = "idemixgen"
	ordererCMD           = "orderer"
	peerCMD              = "peer"
)

func pathExists(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}
	return true
}

// findCmdAtEnv tries to find cmd at the path specified via FabricBinsPathEnvKey
// Returns the full path of cmd if exists; otherwise an empty string
// Example:
//
//	export FAB_BINS=/tmp/fabric/bin/
//	findCmdAtEnv("peer") will return "/tmp/fabric/bin/peer" if exists
func findCmdAtEnv(cmd string) string {
	cmdPath := path.Join(os.Getenv(FabricBinsPathEnvKey), cmd)
	if !pathExists(cmdPath) {
		// cmd does not exist in folder provided via FabricBinsPathEnvKey
		return ""
	}

	return cmdPath
}

// findCMD returns the full path of cmd. It first tries to find the cmd at the path specified via FabricBinsPathEnvKey;
// otherwise the builder function is used.
func findOrBuild(cmd string, builder func() string) string {
	cmdPath := findCmdAtEnv(cmd)
	if len(cmdPath) == 0 {
		cmdPath = builder()
	}

	logger.Debugf("Found %s => %s", cmd, cmdPath)
	return cmdPath
}
