/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"errors"

	config2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/client"
)

// NewWebClientConfigFromFSC returns a web configuration from an FSC node configuration file
func NewWebClientConfigFromFSC(confDir string) (*client.Config, error) {
	config := &client.Config{}
	configProvider, err := config2.NewProvider(confDir)
	if err != nil {
		return nil, err
	}
	config.Host = configProvider.GetString("fsc.web.address")
	if configProvider.GetBool("fsc.web.tls.enabled") {
		config.TLSCertPath = configProvider.GetPath("fsc.web.tls.cert.file")
		config.TLSKeyPath = configProvider.GetPath("fsc.web.tls.key.file")
		if len(config.TLSCertPath) == 0 {
			return nil, errors.New("web configuration must have sc.web.tls.cert.file with file key defined")
		}
		config.CACertPath = configProvider.TranslatePath(config.TLSCertPath)
	}
	return config, nil
}
