/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

func TestValidateConfigCommand(t *testing.T) { //nolint:paralleltest
	gt := gomega.NewWithT(t)

	cli, err := gexec.Build("github.com/hyperledger-labs/fabric-smart-client/cmd/fsccli")
	gt.Expect(err).NotTo(gomega.HaveOccurred())
	defer gexec.CleanupBuildArtifacts()

	confPath := t.TempDir()
	gt.Expect(os.WriteFile(filepath.Join(confPath, "core.yaml"), []byte(validCLIConfig(t)), 0o600)).NotTo(gomega.HaveOccurred())

	out := gbytes.NewBuffer()
	errOut := gbytes.NewBuffer()
	session, err := gexec.Start(exec.Command(cli, "validate", "config", "--config-path", confPath), out, errOut)
	gt.Expect(err).NotTo(gomega.HaveOccurred())
	gt.Eventually(session, time.Minute).Should(gexec.Exit(0))
	gt.Expect(out).To(gbytes.Say("configuration is valid"))
	gt.Expect(out).To(gbytes.Say("validated fsc.grpc server configuration"))
}

func validCLIConfig(t *testing.T) string {
	t.Helper()

	cert := repoFixturePath(t, "integration", "fsc", "pingpong", "testdata", "fsc", "crypto", "peerOrganizations", "fsc.example.com", "peers", "initiator.fsc.example.com", "tls", "server.crt")
	key := repoFixturePath(t, "integration", "fsc", "pingpong", "testdata", "fsc", "crypto", "peerOrganizations", "fsc.example.com", "peers", "initiator.fsc.example.com", "tls", "server.key")

	return "" +
		"fsc:\n" +
		"  id: cli-node\n" +
		"  grpc:\n" +
		"    enabled: true\n" +
		"    address: 127.0.0.1:7051\n" +
		"    tls:\n" +
		"      enabled: true\n" +
		"      cert:\n" +
		"        file: " + cert + "\n" +
		"      key:\n" +
		"        file: " + key + "\n"
}

func repoFixturePath(t *testing.T, elems ...string) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve current file")
	}

	parts := append([]string{filepath.Dir(currentFile), "..", ".."}, elems...)
	return filepath.Clean(filepath.Join(parts...))
}
