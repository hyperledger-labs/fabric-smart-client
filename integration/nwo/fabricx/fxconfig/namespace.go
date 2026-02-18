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
	"strings"

	"github.com/onsi/gomega"
)

const (
	FabricBinsPathEnvKey = "FAB_BINS"
	fxconfigCMD          = "fxconfig"
)

type OrdererConfig struct {
	Address   string
	TLSConfig TLSConfig
}

type MSPConfig struct {
	ConfigPath string
	LocalMspID string
}

type TLSConfig struct {
	Enabled        bool
	RootCerts      []string
	ClientKeyPath  string
	ClientCertPath string
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
	return []string{
		"namespace", "create", n.Name,
		"--channel", n.Channel,
		"--policy-ecdsa-threshold", n.EndorserPKPath,
	}
}

func (n *NamespaceCommon) Env() []string {
	// msp
	env := []string{
		"FXCONFIG_MSP_LOCALMSPID=" + n.MSPConfig.LocalMspID,
		"FXCONFIG_MSP_CONFIGPATH=" + n.MSPConfig.ConfigPath,
	}

	// orderer
	env = append(env, "FXCONFIG_ORDERER_ADDRESS="+n.OrdererConfig.Address)
	if n.OrdererConfig.TLSConfig.Enabled {
		rootCerts := strings.Join(n.OrdererConfig.TLSConfig.RootCerts, ",")
		env = append(env,
			"FXCONFIG_ORDERER_TLS_ENABELD=true",
			"FXCONFIG_ORDERER_TLS_ROOTCERTS="+rootCerts,
		)
	}

	return env
}

func (n *CreateNamespace) SessionName() string {
	return fmt.Sprintf("%s-createnamespace", fxconfigCMD)
}

type UpdateNamespace struct {
	NamespaceCommon

	Version int
}

func (n *UpdateNamespace) Args() []string {
	return []string{
		"namespace", "update", n.Name,
		"--channel", n.Channel,
		"--version", strconv.Itoa(n.Version),
		"--policy-ecdsa-threshold", n.EndorserPKPath,
	}
}

func (n *UpdateNamespace) SessionName() string {
	return fmt.Sprintf("%s-updatenamespace", fxconfigCMD)
}

type ListNamespaces struct {
	QueryServiceEndpoint string
}

func (n *ListNamespaces) Args() []string {
	return []string{"namespace", "list"}
}

func (n *ListNamespaces) Env() []string {
	return []string{"FXCONFIG_QUERIES_ADDRESS=" + n.QueryServiceEndpoint}
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
