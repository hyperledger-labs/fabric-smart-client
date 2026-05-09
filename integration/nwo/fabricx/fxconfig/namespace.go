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

type NotificationsConfig struct {
	Address   string
	TLSConfig TLSConfig
}

type QueryConfig struct {
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

	// Policy is the raw fxconfig policy string, for example:
	//   "AND('Org1MSP.member','Org2MSP.member')"
	// or
	//   "threshold:/path/to/policy.pem"
	Policy string

	MSPConfig           MSPConfig
	OrdererConfig       OrdererConfig
	NotificationsConfig NotificationsConfig
}

type CreateNamespace struct {
	NamespaceCommon
}

func (n *CreateNamespace) Args() []string {
	return []string{
		"namespace", "create", n.Name,
		"--policy=" + n.Policy,
		"--endorse",
		"--submit",
		"--wait",
	}
}

func (n *NamespaceCommon) Env() []string {
	// msp
	env := []string{
		"FXCONFIG_MSP_LOCALMSPID=" + n.MSPConfig.LocalMspID,
		"FX_MSP_LOCALMSPID=" + n.MSPConfig.LocalMspID,
		"FXCONFIG_MSP_CONFIGPATH=" + n.MSPConfig.ConfigPath,
		"FX_MSP_CONFIGPATH=" + n.MSPConfig.ConfigPath,
		"FXCONFIG_ORDERER_CHANNEL=" + n.Channel,
		"FX_ORDERER_CHANNEL=" + n.Channel,
	}

	// orderer
	env = append(env, "FXCONFIG_ORDERER_ADDRESS="+n.OrdererConfig.Address)
	env = append(env, "FX_ORDERER_ADDRESS="+n.OrdererConfig.Address)
	env = append(env, "FXCONFIG_ORDERERS_ADDRESS="+n.OrdererConfig.Address)
	env = append(env, "FX_ORDERERS_ADDRESS="+n.OrdererConfig.Address)

	// notifications
	env = append(env, "FXCONFIG_NOTIFICATIONS_ADDRESS="+n.NotificationsConfig.Address)
	env = append(env, "FX_NOTIFICATIONS_ADDRESS="+n.NotificationsConfig.Address)
	env = append(env, "FXCONFIG_NOTIFICATION_ADDRESS="+n.NotificationsConfig.Address)
	env = append(env, "FX_NOTIFICATION_ADDRESS="+n.NotificationsConfig.Address)

	// global TLS (applies to orderer, notifications, etc.)
	if n.OrdererConfig.TLSConfig.Enabled {
		rootCerts := strings.Join(n.OrdererConfig.TLSConfig.RootCerts, ",")
		env = append(env,
			"FXCONFIG_TLS_ENABLED=true",
			"FX_TLS_ENABLED=true",
			"FXCONFIG_TLS_ROOTCERTS="+rootCerts,
			"FX_TLS_ROOTCERTS="+rootCerts,
		)
		if n.OrdererConfig.TLSConfig.ClientCertPath != "" {
			env = append(env,
				"FXCONFIG_TLS_CLIENTCERT="+n.OrdererConfig.TLSConfig.ClientCertPath,
				"FX_TLS_CLIENTCERT="+n.OrdererConfig.TLSConfig.ClientCertPath,
				"FXCONFIG_TLS_CLIENTSIDEAUTH=true",
				"FX_TLS_CLIENTSIDEAUTH=true",
				"FXCONFIG_TLS_CLIENTAUTHREQUIRED=true",
				"FX_TLS_CLIENTAUTHREQUIRED=true",
				"FXCONFIG_ORDERER_TLS_CLIENTCERT="+n.OrdererConfig.TLSConfig.ClientCertPath,
				"FX_ORDERER_TLS_CLIENTCERT="+n.OrdererConfig.TLSConfig.ClientCertPath,
				"FXCONFIG_ORDERER_TLS_CLIENTSIDEAUTH=true",
				"FX_ORDERER_TLS_CLIENTSIDEAUTH=true",
				"FXCONFIG_ORDERER_TLS_CLIENTAUTHREQUIRED=true",
				"FX_ORDERER_TLS_CLIENTAUTHREQUIRED=true",
				"FXCONFIG_NOTIFICATIONS_TLS_CLIENTCERT="+n.NotificationsConfig.TLSConfig.ClientCertPath,
				"FX_NOTIFICATIONS_TLS_CLIENTCERT="+n.NotificationsConfig.TLSConfig.ClientCertPath,
				"FXCONFIG_NOTIFICATIONS_TLS_CLIENTSIDEAUTH=true",
				"FX_NOTIFICATIONS_TLS_CLIENTSIDEAUTH=true",
				"FXCONFIG_NOTIFICATIONS_TLS_CLIENTAUTHREQUIRED=true",
				"FX_NOTIFICATIONS_TLS_CLIENTAUTHREQUIRED=true",
			)
		}
		if n.OrdererConfig.TLSConfig.ClientKeyPath != "" {
			env = append(env,
				"FXCONFIG_TLS_CLIENTKEY="+n.OrdererConfig.TLSConfig.ClientKeyPath,
				"FX_TLS_CLIENTKEY="+n.OrdererConfig.TLSConfig.ClientKeyPath,
				"FXCONFIG_ORDERER_TLS_CLIENTKEY="+n.OrdererConfig.TLSConfig.ClientKeyPath,
				"FX_ORDERER_TLS_CLIENTKEY="+n.OrdererConfig.TLSConfig.ClientKeyPath,
				"FXCONFIG_NOTIFICATIONS_TLS_CLIENTKEY="+n.NotificationsConfig.TLSConfig.ClientKeyPath,
				"FX_NOTIFICATIONS_TLS_CLIENTKEY="+n.NotificationsConfig.TLSConfig.ClientKeyPath,
			)
		}
		// Explicitly set orderer and notifications root certs to avoid overrides
		env = append(env,
			"FXCONFIG_ORDERER_TLS_ENABLED=true",
			"FX_ORDERER_TLS_ENABLED=true",
			"FXCONFIG_ORDERER_TLS_ROOTCERTS="+rootCerts,
			"FX_ORDERER_TLS_ROOTCERTS="+rootCerts,
			"FXCONFIG_NOTIFICATIONS_TLS_ENABLED=true",
			"FX_NOTIFICATIONS_TLS_ENABLED=true",
			"FXCONFIG_NOTIFICATIONS_TLS_ROOTCERTS="+strings.Join(n.NotificationsConfig.TLSConfig.RootCerts, ","),
			"FX_NOTIFICATIONS_TLS_ROOTCERTS="+strings.Join(n.NotificationsConfig.TLSConfig.RootCerts, ","),
		)
	} else {
		env = append(env,
			"FXCONFIG_TLS_ENABLED=false",
			"FX_TLS_ENABLED=false",
			"FXCONFIG_ORDERER_TLS_ENABLED=false",
			"FX_ORDERER_TLS_ENABLED=false",
			"FXCONFIG_NOTIFICATIONS_TLS_ENABLED=false",
			"FX_NOTIFICATIONS_TLS_ENABLED=false",
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
		"--version", strconv.Itoa(n.Version),
		"--policy=" + n.Policy,
		"--endorse",
		"--submit",
		"--wait",
	}
}

func (n *UpdateNamespace) SessionName() string {
	return fmt.Sprintf("%s-updatenamespace", fxconfigCMD)
}

type ListNamespaces struct {
	QueryConfig QueryConfig
}

func (n *ListNamespaces) Args() []string {
	return []string{"namespace", "list"}
}

func (n *ListNamespaces) Env() []string {
	env := []string{
		"FXCONFIG_QUERIES_ADDRESS=" + n.QueryConfig.Address,
		"FX_QUERIES_ADDRESS=" + n.QueryConfig.Address,
		"FXCONFIG_QUERY_ADDRESS=" + n.QueryConfig.Address,
		"FX_QUERY_ADDRESS=" + n.QueryConfig.Address,
		"FXCONFIG_QUERYSERVICE_ADDRESS=" + n.QueryConfig.Address,
		"FX_QUERYSERVICE_ADDRESS=" + n.QueryConfig.Address,
	}

	if n.QueryConfig.TLSConfig.Enabled {
		rootCerts := strings.Join(n.QueryConfig.TLSConfig.RootCerts, ",")
		env = append(env,
			"FXCONFIG_TLS_ENABLED=true",
			"FX_TLS_ENABLED=true",
			"FXCONFIG_TLS_ROOTCERTS="+rootCerts,
			"FX_TLS_ROOTCERTS="+rootCerts,
			"FXCONFIG_QUERIES_TLS_ENABLED=true",
			"FX_QUERIES_TLS_ENABLED=true",
			"FXCONFIG_QUERIES_TLS_ROOTCERTS="+rootCerts,
			"FX_QUERIES_TLS_ROOTCERTS="+rootCerts,
			"FXCONFIG_QUERY_TLS_ENABLED=true",
			"FX_QUERY_TLS_ENABLED=true",
			"FXCONFIG_QUERY_TLS_ROOTCERTS="+rootCerts,
			"FX_QUERY_TLS_ROOTCERTS="+rootCerts,
		)
		if n.QueryConfig.TLSConfig.ClientCertPath != "" {
			env = append(env,
				"FXCONFIG_TLS_CLIENTCERT="+n.QueryConfig.TLSConfig.ClientCertPath,
				"FX_TLS_CLIENTCERT="+n.QueryConfig.TLSConfig.ClientCertPath,
				"FXCONFIG_TLS_CLIENTSIDEAUTH=true",
				"FX_TLS_CLIENTSIDEAUTH=true",
				"FXCONFIG_TLS_CLIENTAUTHREQUIRED=true",
				"FX_TLS_CLIENTAUTHREQUIRED=true",
				"FXCONFIG_QUERIES_TLS_CLIENTCERT="+n.QueryConfig.TLSConfig.ClientCertPath,
				"FX_QUERIES_TLS_CLIENTCERT="+n.QueryConfig.TLSConfig.ClientCertPath,
				"FXCONFIG_QUERIES_TLS_CLIENTSIDEAUTH=true",
				"FX_QUERIES_TLS_CLIENTSIDEAUTH=true",
				"FXCONFIG_QUERIES_TLS_CLIENTAUTHREQUIRED=true",
				"FX_QUERIES_TLS_CLIENTAUTHREQUIRED=true",
				"FXCONFIG_QUERY_TLS_CLIENTCERT="+n.QueryConfig.TLSConfig.ClientCertPath,
				"FX_QUERY_TLS_CLIENTCERT="+n.QueryConfig.TLSConfig.ClientCertPath,
				"FXCONFIG_QUERY_TLS_CLIENTSIDEAUTH=true",
				"FX_QUERY_TLS_CLIENTSIDEAUTH=true",
				"FXCONFIG_QUERY_TLS_CLIENTAUTHREQUIRED=true",
				"FX_QUERY_TLS_CLIENTAUTHREQUIRED=true",
			)
		}
		if n.QueryConfig.TLSConfig.ClientKeyPath != "" {
			env = append(env,
				"FXCONFIG_TLS_CLIENTKEY="+n.QueryConfig.TLSConfig.ClientKeyPath,
				"FX_TLS_CLIENTKEY="+n.QueryConfig.TLSConfig.ClientKeyPath,
				"FXCONFIG_QUERIES_TLS_CLIENTKEY="+n.QueryConfig.TLSConfig.ClientKeyPath,
				"FX_QUERIES_TLS_CLIENTKEY="+n.QueryConfig.TLSConfig.ClientKeyPath,
				"FXCONFIG_QUERY_TLS_CLIENTKEY="+n.QueryConfig.TLSConfig.ClientKeyPath,
				"FX_QUERY_TLS_CLIENTKEY="+n.QueryConfig.TLSConfig.ClientKeyPath,
			)
		}
	}

	return env
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
