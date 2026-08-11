/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ccaas

import "testing"

func TestContainerEnvSetsLocalMSPID(t *testing.T) { //nolint:paralleltest
	env := containerEnv(ContainerSpec{
		CCID:          "asset_transfer:abc123",
		ServerAddress: "0.0.0.0:9999",
		MSPID:         "Org1MSP",
	})

	want := []string{
		"CHAINCODE_ID=asset_transfer:abc123",
		"CHAINCODE_SERVER_ADDRESS=0.0.0.0:9999",
		"CHAINCODE_TLS=false",
		"CORE_PEER_LOCALMSPID=Org1MSP",
	}
	if len(env) != len(want) {
		t.Fatalf("got %d vars %v, want %d", len(env), env, len(want))
	}
	for i := range want {
		if env[i] != want[i] {
			t.Errorf("env[%d] = %q, want %q", i, env[i], want[i])
		}
	}
}
