/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package serve runs a chaincode either as a classic shim chaincode or as a
// Chaincode-as-a-Service (CCaaS) gRPC server, selected by the environment.
package serve

import (
	"os"

	"github.com/hyperledger/fabric-chaincode-go/v2/shim"
)

// ServerConfig holds the CCaaS server settings resolved from the environment.
type ServerConfig struct {
	CCID    string
	Address string
}

// ServerConfigFromEnv reads CHAINCODE_ID and CHAINCODE_SERVER_ADDRESS. The
// bool result is true only when both are set, i.e. CCaaS mode is active.
func ServerConfigFromEnv() (ServerConfig, bool) {
	cfg := ServerConfig{
		CCID:    os.Getenv("CHAINCODE_ID"),
		Address: os.Getenv("CHAINCODE_SERVER_ADDRESS"),
	}
	return cfg, cfg.CCID != "" && cfg.Address != ""
}

// Serve runs cc as a CCaaS gRPC server when the environment selects it,
// otherwise via the classic shim.Start path. The same binary works in both
// modes with no code change.
func Serve(cc shim.Chaincode) error {
	if cfg, ok := ServerConfigFromEnv(); ok {
		server := &shim.ChaincodeServer{
			CCID:     cfg.CCID,
			Address:  cfg.Address,
			CC:       cc,
			TLSProps: shim.TLSProperties{Disabled: true},
		}
		return server.Start()
	}
	return shim.Start(cc)
}
