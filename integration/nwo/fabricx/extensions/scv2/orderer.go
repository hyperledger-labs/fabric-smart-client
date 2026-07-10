/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scv2

import (
	"os"
)

func generateMockOrdererConfigFile(configPath string) error {
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
  - msp-id: OrdererMSP
    msp-dir: /root/artifacts/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp
`
	return os.WriteFile(configPath, []byte(configContent), 0o600)
}
