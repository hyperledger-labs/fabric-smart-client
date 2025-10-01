/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fxconfig

import (
	"fmt"
	"os"
	"path"
	"strconv"

	"github.com/onsi/gomega"
)

const (
	FabricBinsPathEnvKey = "FAB_BINS"
	fxconfigCMD          = "fxconfig"
)

type OrdererConfig struct {
	Endpoint string
	Tls      bool
	CAFile   string
}

type MSPConfig struct {
	Path string
	Name string
}

type NamespaceCommon struct {
	Name    string
	Channel string

	EndorserPKPath string

	MSPConfig     MSPConfig
	OrdererConfig OrdererConfig
}

type CreateNamespace struct {
	NamespaceCommon
}

func (n *CreateNamespace) Args() []string {
	var args []string

	args = append(args, "namespace", "create", n.Name)
	args = append(args, "--channel", n.Channel)

	args = append(args, "--orderer", n.OrdererConfig.Endpoint)
	if n.OrdererConfig.Tls {
		args = append(args, "--tls")
		args = append(args, "--cafile", n.OrdererConfig.CAFile)
	}

	args = append(args, "--mspConfigPath", n.MSPConfig.Path)
	args = append(args, "--mspID", n.MSPConfig.Name)

	if len(n.EndorserPKPath) > 0 {
		args = append(args, "--pk", n.EndorserPKPath)
	}

	return args
}

func (n *CreateNamespace) SessionName() string {
	return fmt.Sprintf("%s-createnamespace", fxconfigCMD)
}

type UpdateNamespace struct {
	NamespaceCommon

	Version int
}

func (n *UpdateNamespace) Args() []string {
	var args []string

	args = append(args, "namespace", "update")
	args = append(args, n.Name)
	args = append(args, "--channel", n.Channel)

	args = append(args, "--orderer", n.OrdererConfig.Endpoint)
	if n.OrdererConfig.Tls {
		args = append(args, "--tls")
		args = append(args, "--cafile", n.OrdererConfig.CAFile)
	}

	args = append(args, "--mspConfigPath", n.MSPConfig.Path)
	args = append(args, "--mspID", n.MSPConfig.Name)

	args = append(args, "--version", strconv.Itoa(n.Version))
	if len(n.EndorserPKPath) > 0 {
		args = append(args, "--pk", n.EndorserPKPath)
	}

	return args
}

func (n *UpdateNamespace) SessionName() string {
	return fmt.Sprintf("%s-updatenamespace", fxconfigCMD)
}

type ListNamespaces struct {
	QueryServiceEndpoint string
}

func (n *ListNamespaces) Args() []string {
	var args []string

	args = append(args, "namespace", "list")
	args = append(args, "--endpoint", n.QueryServiceEndpoint)

	return args
}

func (n *ListNamespaces) SessionName() string {
	return fmt.Sprintf("%s-listnamespaces", fxconfigCMD)
}

// CMDPath returns the full path of fxconfig at path specified via FabricBinsPathEnvKey.
func CMDPath() string {
	cmdPath := findCmdAtEnv(fxconfigCMD)
	gomega.Expect(cmdPath).NotTo(gomega.Equal(""), "could not find %s in %s directory %s", fxconfigCMD, FabricBinsPathEnvKey, os.Getenv(FabricBinsPathEnvKey))
	return cmdPath
}

func pathExists(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}
	return true
}

// findCmdAtEnv tries to find cmd at the path specified via FabricBinsPathEnvKey
// Returns the full path of cmd if exists; otherwise an empty string
// Example:
//
//	export FAB_BINS=/tmp/fabric/bin/
//	findCmdAtEnv("peer") will return "/tmp/fabric/bin/peer" if exists
func findCmdAtEnv(cmd string) string {
	cmdPath := path.Join(os.Getenv(FabricBinsPathEnvKey), cmd)
	if !pathExists(cmdPath) {
		// cmd does not exist in folder provided via FabricBinsPathEnvKey
		return ""
	}

	return cmdPath
}
