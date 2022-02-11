/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fpc

import (
	"encoding/json"
	. "github.com/onsi/gomega"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	fscRoot            = "github.com/hyperledger-labs/fabric-smart-client"
	relExternalBuilder = "integration/nwo/fabric/fpc/externalbuilders/chaincode_server"
)

type goDep struct {
	Path  string
	Dir   string
	Error string
}

type goModule struct {
	Path    string
	Version string
}

type goReplace struct {
	Old goModule
	New goModule
}

type mods struct {
	Module  goModule
	Require []goModule
	Replace []goReplace
}

func (m *mods) requiresModule(path string) (bool, *goModule) {
	for _, mod := range m.Require {
		if mod.Path == path {
			return true, &mod
		}
	}
	return false, nil
}

func (m *mods) isReplacedModule(path string) (bool, *goModule) {
	for _, r := range m.Replace {
		if r.Old.Path == path {
			return true, &r.New
		}
	}
	return false, nil
}

func buildersExist(path string) error {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return err
	}

	// TODO let's also check if the scripts are there

	logger.Debugf("found external builder at %s\n", path)
	return nil
}

func externalBuilderFromGoPath() string {
	cmd := exec.Command("go", "env", "GOPATH")
	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	out, err := cmd.Output()
	Expect(err).ToNot(HaveOccurred())
	goPathDir := filepath.Clean(strings.TrimSpace(string(out)))

	path := filepath.Join(goPathDir, "src", fscRoot, filepath.Join(strings.Split(relExternalBuilder, "/")...))
	if err := buildersExist(path); err == nil {
		return path
	}

	return ""
}

func externalBuilderFromGoModule() string {
	cmd := exec.Command("go", "mod", "edit", "-json")
	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	out, _ := cmd.Output()

	if strings.TrimSpace(string(out)) == "" {
		// not a go module
		return ""
	}

	mods := &mods{}
	err := json.Unmarshal(out, mods)
	Expect(err).ToNot(HaveOccurred())

	// let's check if we are in FSC
	if mods.Module.Path == fscRoot {
		cmd := exec.Command("go", "env", "GOMOD")
		cmd.Env = append(os.Environ(), "GO111MODULE=on")
		out, err := cmd.Output()
		Expect(err).ToNot(HaveOccurred())
		moduleRootDir := filepath.Dir(string(out))

		path := filepath.Join(moduleRootDir, filepath.Join(strings.Split(relExternalBuilder, "/")...))
		if err := buildersExist(path); err == nil {
			// found external builders in our project
			return path
		}
	}

	// check if FSC is a go mod dependency, let's see if it is in the cache
	if requires, _ := mods.requiresModule(fscRoot); !requires {
		// FSC is not a go dependency
		return ""
	}

	// check if dep is replaced
	if replaced, mod := mods.isReplacedModule(fscRoot); replaced {
		// search in the replacement path
		path := filepath.Join(mod.Path, filepath.Join(strings.Split(relExternalBuilder, "/")...))
		if err := buildersExist(path); err == nil {
			return path
		}
	}

	// check if we find FSC in mod cache
	cmd = exec.Command("go", "mod", "download", "-json", fscRoot)
	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	out, _ = cmd.Output()

	dep := &goDep{}
	err = json.Unmarshal(out, dep)
	Expect(err).ToNot(HaveOccurred())

	path := filepath.Join(dep.Dir, filepath.Join(strings.Split(relExternalBuilder, "/")...))
	if err := buildersExist(path); err == nil {
		return path
	}

	// we did everything to find FSC external builders, no luck
	return ""
}
