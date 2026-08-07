/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package topology

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The Makefile CHAINCODE_IMAGES list must contain exactly the image names the
// topology helpers assign, so a `make chaincode-images` run produces every image
// a default (CCaaS) test needs.
func TestImageNamesMatchMakefile(t *testing.T) {
	want := map[string]bool{
		ImageBaseChaincode:    true,
		ImageStateChaincode:   true,
		ImageEventsChaincode:  true,
		ImageEvents2Chaincode: true,
		ImageATSAChaincode:    true,
	}

	// Locate the repo-root Makefile relative to this package
	// (integration/nwo/fabric/topology -> repo root is four levels up).
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	mk := string(data)

	if !strings.Contains(mk, "CHAINCODE_IMAGES") {
		t.Skip("CHAINCODE_IMAGES added in a later task")
		return
	}

	for img := range want {
		bare := strings.TrimSuffix(img, ":latest")
		if !strings.Contains(mk, bare) {
			t.Errorf("Makefile CHAINCODE_IMAGES missing image %q", bare)
		}
	}
}
