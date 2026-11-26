/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package evm_test

import (
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/nwo/evm"
)

// TestBesuImagePin checks that the image StartBesu runs is the one
// `make pull-images-evm` pulls. DefaultBesuImage has no registry prefix (Docker
// resolves "hyperledger/besu:..." to docker.io), while the Makefile's pull line
// names the registry, so the comparison adds it back.
func TestBesuImagePin(t *testing.T) {
	t.Parallel()

	makefile, err := os.ReadFile("../../../../Makefile")
	require.NoError(t, err)
	version := regexp.MustCompile(`(?m)^BESU_VERSION \?= (\S+)$`).FindSubmatch(makefile)
	require.NotNil(t, version, "no BESU_VERSION ?= line in the Makefile")
	require.Contains(t, string(makefile), "\tdocker pull docker.io/hyperledger/besu:$(BESU_VERSION)\n")
	require.Equal(t, "docker.io/"+evm.DefaultBesuImage, "docker.io/hyperledger/besu:"+string(version[1]))
}
