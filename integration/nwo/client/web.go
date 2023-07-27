/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"errors"

	config2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/core/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/client/web"
)

// NewWebClientConfigFromFSC returns a web configuration from an FSC node configuration file
func NewWebClientConfigFromFSC(confDir string) (*web.Config, error) {
	config := &web.Config{}
	configProvider, err := config2.NewProvider(confDir)
	if err != nil {
		return nil, err
	}
	config.Host = configProvider.GetString("fsc.web.address")
	if configProvider.GetBool("fsc.web.tls.enabled") {
		tlsRootCertFile := configProvider.GetString("fsc.web.tls.serverRootCert.file")
		if len(tlsRootCertFile) == 0 {
			return nil, errors.New("web configuration must have serverRootCert with file key defined")
		}
		config.CACert = configProvider.TranslatePath(tlsRootCertFile)
		config.TLSCert = configProvider.GetPath("fsc.web.tls.clientCert.file")
		config.TLSKey = configProvider.GetPath("fsc.web.tls.clientKey.file")
	}
	return config, nil
}
