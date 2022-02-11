/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fpc

import (
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestEndToEnd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "External builder test suite")
}

var _ = Describe("Find external builder path", func() {
	var (
		extension Extension
		tmpDir    string
		err       error
	)

	When("calling from FSC project", func() {
		It("should return path", func() {
			p, err := extension.FindExternalBuilder()
			Expect(err).ToNot(HaveOccurred())
			Expect(p).NotTo(BeEmpty())
		})
	})

	When("calling from another go project", func() {

		BeforeEach(func() {
			tmpDir, err = ioutil.TempDir("/tmp", "external-builder-test")
			Expect(err).ToNot(HaveOccurred())

			// switch to our tmp directory
			err = os.Chdir(tmpDir)
			Expect(err).ToNot(HaveOccurred())

			// just unset gopath for the moment
			err = os.Unsetenv("GOPATH")
			Expect(err).ToNot(HaveOccurred())

			cmd := exec.Command("go", "mod", "init", "example.com/test-lab/test")
			_, err := cmd.Output()
			Expect(err).ToNot(HaveOccurred())

			main := `package main

import "github.com/hyperledger-labs/fabric-smart-client/integration"

func main() {
	_ = integration.FPCEchoPort
}
`
			err = os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte(main), 0644)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			err = os.RemoveAll(tmpDir)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return error if not found in go.mod", func() {
			_, err := extension.FindExternalBuilder()
			Expect(err).To(HaveOccurred())
		})

		When("FSC is in go.mod", func() {
			BeforeEach(func() {
				// get FSC in the mod cache
				cmd := exec.Command("go", "mod", "tidy")
				_, err = cmd.Output()
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return the path in mod cache", func() {
				p, err := extension.FindExternalBuilder()
				Expect(err).ToNot(HaveOccurred())
				Expect(p).NotTo(BeEmpty())
			})

		})

		When("FSC is in go mod but replaced", func() {
			BeforeEach(func() {
				// get FSC in the mod cache
				cmd := exec.Command("go", "mod", "tidy")
				_, err = cmd.Output()
				Expect(err).ToNot(HaveOccurred())

				// create builder directory at replacement location
				replacePath := filepath.Join(tmpDir, fscRoot)
				builderPath := filepath.Join(replacePath, relExternalBuilder)
				err = os.MkdirAll(builderPath, 0755)
				Expect(err).ToNot(HaveOccurred())

				cmd = exec.Command("go", "mod", "edit", fmt.Sprintf("-replace=%s=%s", fscRoot, replacePath))
				_, err = cmd.Output()
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return the path", func() {
				p, err := extension.FindExternalBuilder()
				Expect(err).ToNot(HaveOccurred())
				Expect(p).NotTo(BeEmpty())
			})

		})

	})

	When("calling from somewhere else (not in a go module)", func() {
		BeforeEach(func() {
			tmpDir, err = ioutil.TempDir("/tmp", "external-builder-test")
			Expect(err).ToNot(HaveOccurred())

			// switch to our tmp directory
			err = os.Chdir(tmpDir)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			err = os.RemoveAll(tmpDir)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return path on the gopath", func() {
			// let's create our expected external builder path
			err = os.MkdirAll(filepath.Join(tmpDir, "src", fscRoot, relExternalBuilder), 0755)
			Expect(err).ToNot(HaveOccurred())

			os.Setenv("GOPATH", tmpDir)

			p, err := extension.FindExternalBuilder()
			Expect(err).ToNot(HaveOccurred())
			Expect(p).NotTo(BeEmpty())
		})

		It("should return error if not on gopath", func() {
			// we set our gopath to the tmp directory
			os.Unsetenv("GOPATH")

			_, err = extension.FindExternalBuilder()
			Expect(err).To(HaveOccurred())
		})
	})
})
