/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package packager

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPersistence(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Persistence Suite")
}

var _ = Describe("Persistence", func() {
	Describe("FilesystemWriter", func() {
		var (
			testDir string
		)

		BeforeEach(func() {
			var err error
			testDir, err = os.MkdirTemp("", "persistence-test")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			os.RemoveAll(testDir)
		})

		It("writes a file", func() {
			path := filepath.Join(testDir, "write")
			err := WriteFile(testDir, "write", []byte("test"))
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(path)
			Expect(err).NotTo(HaveOccurred())
		})

		When("an empty path is supplied to WriteFile", func() {
			It("returns error", func() {
				err := WriteFile("", "write", []byte("test"))
				Expect(err.Error()).To(Equal("empty path not allowed"))
			})
		})
	})

})
