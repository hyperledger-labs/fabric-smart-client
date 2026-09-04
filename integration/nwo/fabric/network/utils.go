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
	osnadminCMD          = "osnadmin"
)

func pathExists(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}
	return true
}

// binDir returns the directory the network's binaries are looked up in: the one
// named by FabricBinsPathEnvKey, narrowed to BinSubdir when the network sets one.
func (n *Network) binDir() string {
	return path.Join(os.Getenv(FabricBinsPathEnvKey), n.BinSubdir)
}

// findCmdAtEnv tries to find cmd at the path specified via FabricBinsPathEnvKey,
// inside the network's BinSubdir if it declares one.
// Returns the full path of cmd if exists; otherwise an empty string
// Example:
//
//	export FAB_BINS=/tmp/fabric/bin/
//	findCmdAtEnv("peer") will return "/tmp/fabric/bin/peer" if exists
func (n *Network) findCmdAtEnv(cmd string) string {
	cmdPath := path.Join(n.binDir(), cmd)
	if !pathExists(cmdPath) {
		// cmd does not exist in folder provided via FabricBinsPathEnvKey
		return ""
	}

	return cmdPath
}

// findOrBuild returns the full path of cmd. It first tries to find the cmd at the path specified via FabricBinsPathEnvKey;
// otherwise the builder function is used.
func (n *Network) findOrBuild(cmd string, builder func() string) string {
	cmdPath := n.findCmdAtEnv(cmd)
	if len(cmdPath) == 0 {
		cmdPath = builder()
	}

	logger.Debugf("Found %s => %s", cmd, cmdPath)
	return cmdPath
}
