/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package serve

import "testing"

func TestServerConfigFromEnv(t *testing.T) {
	t.Setenv("CHAINCODE_ID", "")
	t.Setenv("CHAINCODE_SERVER_ADDRESS", "")
	if _, ok := ServerConfigFromEnv(); ok {
		t.Fatalf("expected CCaaS mode off when env unset")
	}

	t.Setenv("CHAINCODE_ID", "mycc:abc")
	t.Setenv("CHAINCODE_SERVER_ADDRESS", "0.0.0.0:9999")
	cfg, ok := ServerConfigFromEnv()
	if !ok {
		t.Fatalf("expected CCaaS mode on when both env vars set")
	}
	if cfg.CCID != "mycc:abc" || cfg.Address != "0.0.0.0:9999" {
		t.Fatalf("unexpected config: %+v", cfg)
	}

	t.Setenv("CHAINCODE_SERVER_ADDRESS", "")
	if _, ok := ServerConfigFromEnv(); ok {
		t.Fatalf("expected CCaaS mode off when only CHAINCODE_ID set")
	}
}
