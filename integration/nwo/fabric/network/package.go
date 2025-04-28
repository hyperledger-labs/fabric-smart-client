/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/onsi/gomega"
)

// PackageChaincodeBinary is a helper function to package
// an already built chaincode and write it to the location
// specified by Chaincode.PackageFile.
func PackageChaincodeBinary(c *topology.Chaincode) {
	file, err := os.Create(c.PackageFile)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	defer utils.IgnoreErrorFunc(file.Close)
	writeTarGz(c, file)
}

func writeTarGz(c *topology.Chaincode, w io.Writer) {
	gw := gzip.NewWriter(w)
	tw := tar.NewWriter(gw)
	defer closeAll(tw, gw)

	writeMetadataJSON(tw, c.Path, "binary", c.Label)

	writeCodeTarGz(tw, c.CodeFiles)
}

// packageMetadata holds the path, type, and label for a chaincode package
type packageMetadata struct {
	Path  string `json:"path"`
	Type  string `json:"type"`
	Label string `json:"label"`
}

func writeMetadataJSON(tw *tar.Writer, path, ccType, label string) {
	metadata, err := json.Marshal(&packageMetadata{
		Path:  path,
		Type:  ccType,
		Label: label,
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	// write it to the package as metadata.json
	err = tw.WriteHeader(&tar.Header{
		Name: "metadata.json",
		Size: int64(len(metadata)),
		Mode: 0100644,
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	_, err = tw.Write(metadata)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

func writeCodeTarGz(tw *tar.Writer, codeFiles map[string]string) {
	// create temp file to hold code.tar.gz
	tempfile, err := os.CreateTemp("", "code.tar.gz")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	defer os.Remove(tempfile.Name())

	gzipWriter := gzip.NewWriter(tempfile)
	tarWriter := tar.NewWriter(gzipWriter)

	for source, target := range codeFiles {
		file, err := os.Open(source)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		writeFileToTar(tarWriter, file, target)
		utils.IgnoreErrorFunc(file.Close)
	}

	// close down the inner tar
	closeAll(tarWriter, gzipWriter)

	writeFileToTar(tw, tempfile, "code.tar.gz")
}

func writeFileToTar(tw *tar.Writer, file *os.File, name string) {
	_, err := file.Seek(0, 0)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	fi, err := file.Stat()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	header, err := tar.FileInfoHeader(fi, "")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	header.Name = name
	err = tw.WriteHeader(header)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	_, err = io.Copy(tw, file)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

func closeAll(closers ...io.Closer) {
	for _, c := range closers {
		gomega.Expect(c.Close()).To(gomega.Succeed())
	}
}
