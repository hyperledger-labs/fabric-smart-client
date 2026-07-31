/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ccaas

import (
	"strings"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
)

func TestDefaultExtensionMissingImage(t *testing.T) {
	ext := DefaultExtension{Inspect: func(string) (bool, error) { return false, nil }}
	err := ext.EnsureImage(&topology.Chaincode{Image: "fsc-cc/base:latest"})
	if err == nil {
		t.Fatal("expected error for missing image")
	}
	if !strings.Contains(err.Error(), "fsc-cc/base:latest") || !strings.Contains(err.Error(), "make chaincode-images") {
		t.Fatalf("error must name image and make target: %v", err)
	}
}

func TestDefaultExtensionImagePresent(t *testing.T) {
	ext := DefaultExtension{Inspect: func(string) (bool, error) { return true, nil }}
	if err := ext.EnsureImage(&topology.Chaincode{Image: "fsc-cc/base:latest"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	env, mounts, err := ext.ContainerEnv(&topology.Chaincode{})
	if err != nil || env != nil || mounts != nil {
		t.Fatalf("default ContainerEnv must be empty: env=%v mounts=%v err=%v", env, mounts, err)
	}
}
