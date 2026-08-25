/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scv2

import (
	"fmt"
	"os"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/network"
)

// consenter is one identity the mock ordering service signs blocks as. The
// fabric-x committer container binds a single ordering endpoint, so the BFT
// client's 3f+1 consenters are distinguished by their MSP identity alone.
type consenter struct {
	MSPID  string
	MSPDir string
}

// ordererConsenters resolves one consenter identity per orderer in the topology.
func ordererConsenters(n *network.Network) []consenter {
	consenters := make([]consenter, 0, len(n.Orderers))
	for _, o := range n.Orderers {
		consenters = append(consenters, consenter{
			MSPID:  n.Organization(o.Organization).MSPID,
			MSPDir: containerOrdererMSPDir(n, o),
		})
	}

	return consenters
}

// generateMockOrdererConfigFile creates a mock orderer configuration.
// NOTE: This is a simplified mock configuration. The consenter-msp-identities
// are hardcoded and must match the actual network topology.
func generateMockOrdererConfigFile(configPath string, consenters []consenter) error {
	configContent := `
logging:
  logSpec: info:grpc=error
  format: >-
    %{color}%{time:2006-01-02 15:04:05.000 MST} [%{module}] %{shortfunc}
    -> %{level:.4s}%{color:reset} %{message}
server:
  endpoint: :7050
  tls:
    mode: none
    cert-path: /some/server.crt
    key-path: /some/server.key
    ca-cert-paths:
      - /some/CA-cert.pem
block-size: 1024
block-timeout: 30s
out-block-capacity: 1024
payload-cache-size: 1024
send-genesis-block: true
artifacts-path:

# note that genesis-block-path and consenter-msp-identities cannot be set via env var,
# as they must be set via the config yaml in order to override via env vars
genesis-block-path: /root/artifacts/config-block.pb.bin
consenter-msp-identities:
`
	for _, c := range consenters {
		configContent += fmt.Sprintf("  - msp-id: %s\n    msp-dir: %s\n", c.MSPID, c.MSPDir)
	}

	return os.WriteFile(configPath, []byte(configContent), 0o600)
}
