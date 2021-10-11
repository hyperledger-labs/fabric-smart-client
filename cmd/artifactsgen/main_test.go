/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package artifactsgen

import (
	"bytes"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

func TestArtifactsGen(t *testing.T) {
	RegisterFailHandler(func(message string, callerSkip ...int) {
		panic(message)
	})

	cli, err := gexec.Build("github.com/hyperledger-labs/fabric-smart-client/cmd/cli")
	Expect(err).NotTo(HaveOccurred())
	defer gexec.CleanupBuildArtifacts()

	tmpDir, err := ioutil.TempDir("", t.Name())
	Expect(err).NotTo(HaveOccurred())

	defer os.RemoveAll(tmpDir)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	topologyFolder := filepath.Join("testdata", "fabric_iou.yaml")
	session, err := gexec.Start(exec.Command(cli, "artifactsgen", "gen", "-t", topologyFolder, "-o", tmpDir), stdout, stderr)
	Expect(err).NotTo(HaveOccurred())
	Eventually(session, time.Second*10).Should(gexec.Exit(0))

	entries, err := os.ReadDir(tmpDir)
	Expect(err).NotTo(HaveOccurred())

	var stringEntries []string
	for _, entry := range entries {
		stringEntries = append(stringEntries, entry.Name())
	}

	Expect(stringEntries).To(Equal([]string{"fabric.default", "fsc"}))

}
